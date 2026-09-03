package server

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/mopga/mythblog/internal/content"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxImageDimension = 1920
const maxDecodedPixels = 25_000_000

type mediaFailure struct {
	status        int
	code, message string
}

var errStorageDelete = errors.New("object storage delete failed")

func (a *App) uploadMedia(w http.ResponseWriter, r *http.Request) {
	media, failure := a.createMedia(w, r)
	if failure != nil {
		writeAPIError(w, failure.status, failure.code, failure.message, nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"media": media})
}

func (a *App) createMedia(w http.ResponseWriter, r *http.Request) (content.Media, *mediaFailure) {
	a.mediaMu.Lock()
	defer a.mediaMu.Unlock()
	limit := a.cfg.MaxUploadBytes
	if limit < 1 {
		limit = 10 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
	if err := r.ParseMultipartForm(limit); err != nil {
		return content.Media{}, &mediaFailure{422, "validation_error", "multipart body invalid or too large"}
	}
	articleID, err := strconv.ParseInt(r.FormValue("article_id"), 10, 64)
	if err != nil || articleID < 1 {
		return content.Media{}, &mediaFailure{422, "validation_error", "article_id invalid"}
	}
	article, err := a.content.GetByID(r.Context(), articleID)
	if errors.Is(err, content.ErrNotFound) {
		return content.Media{}, &mediaFailure{404, "not_found", "article not found"}
	}
	if err != nil {
		return content.Media{}, &mediaFailure{500, "internal_error", "database error"}
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return content.Media{}, &mediaFailure{422, "validation_error", "image file required"}
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return content.Media{}, &mediaFailure{422, "validation_error", "image too large"}
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return content.Media{}, &mediaFailure{422, "validation_error", "unsupported image"}
	}
	imageConfig, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return content.Media{}, &mediaFailure{422, "validation_error", "image cannot be decoded"}
	}
	if imageConfig.Width < 1 || imageConfig.Height < 1 || imageConfig.Width > maxDecodedPixels/imageConfig.Height {
		return content.Media{}, &mediaFailure{422, "validation_error", "image dimensions too large"}
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return content.Media{}, &mediaFailure{422, "validation_error", "image cannot be decoded"}
	}
	processed, err := processImage(decoded)
	if err != nil {
		return content.Media{}, &mediaFailure{422, "validation_error", "image processing failed"}
	}
	key := "articles/" + article.Slug + "/" + uuid.NewString() + ".jpg"
	publicURL, err := a.storage.Put(r.Context(), key, processed, "image/jpeg")
	if err != nil {
		return content.Media{}, &mediaFailure{502, "storage_error", "object storage failed"}
	}
	sourceURL := strings.TrimSpace(r.FormValue("source_url"))
	if sourceURL != "" {
		u, parseErr := url.ParseRequestURI(sourceURL)
		if parseErr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			_ = a.storage.Delete(r.Context(), key)
			return content.Media{}, &mediaFailure{422, "validation_error", "source_url invalid"}
		}
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	cover, _ := strconv.ParseBool(r.FormValue("is_cover"))
	media, err := a.content.CreateMedia(r.Context(), content.MediaInput{ArticleID: articleID, Type: "image", URL: publicURL, ObjectKey: key, SourceURL: sourceURL, Caption: r.FormValue("caption"), AltText: r.FormValue("alt_text"), License: r.FormValue("license"), IsCover: cover, SortOrder: sortOrder})
	if err != nil {
		_ = a.storage.Delete(r.Context(), key)
		return content.Media{}, &mediaFailure{500, "internal_error", "media metadata failed"}
	}
	return media, nil
}

func processImage(src image.Image) ([]byte, error) {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width < 1 || height < 1 {
		return nil, errors.New("empty image")
	}
	newW, newH := width, height
	if width > maxImageDimension || height > maxImageDimension {
		scale := float64(maxImageDimension) / float64(width)
		if height > width {
			scale = float64(maxImageDimension) / float64(height)
		}
		newW = int(float64(width) * scale)
		newH = int(float64(height) * scale)
	}
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	var out bytes.Buffer
	err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85})
	return out.Bytes(), err
}

func (a *App) removeMedia(r *http.Request, id int64) *mediaFailure {
	a.mediaMu.Lock()
	defer a.mediaMu.Unlock()
	media, err := a.content.GetMedia(r.Context(), id)
	if errors.Is(err, content.ErrNotFound) {
		return &mediaFailure{404, "not_found", "media not found"}
	}
	if err != nil {
		return &mediaFailure{500, "internal_error", "database error"}
	}
	if err = a.storage.Delete(r.Context(), media.ObjectKey); err != nil {
		return &mediaFailure{502, "storage_error", "object storage delete failed"}
	}
	if err = a.content.DeleteMedia(r.Context(), id); err != nil {
		return &mediaFailure{500, "internal_error", "metadata delete failed"}
	}
	return nil
}

func (a *App) deleteArticleWithMedia(r *http.Request, id int64) error {
	a.mediaMu.Lock()
	defer a.mediaMu.Unlock()
	pending, err := a.content.DeleteAndQueueMedia(r.Context(), id)
	if err != nil {
		return err
	}
	for _, item := range pending {
		if err = a.storage.Delete(r.Context(), item.ObjectKey); err != nil {
			return fmt.Errorf("%w: %v", errStorageDelete, err)
		}
		if err = a.content.CompleteObjectDeletion(r.Context(), item.ID); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) deleteMedia(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeAPIError(w, 400, "invalid_id", "media id invalid", nil)
		return
	}
	if failure := a.removeMedia(r, id); failure != nil {
		writeAPIError(w, failure.status, failure.code, failure.message, nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
