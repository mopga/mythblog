package server_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mopga/mythblog/internal/database"
	"github.com/mopga/mythblog/internal/server"
	"github.com/mopga/mythblog/internal/storage"
)

func TestMediaUploadProcessingAndDelete(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "oddity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	memory := storage.NewMemory("https://media.test")
	h := server.NewWithStorage(testConfigWithMax(1<<20), db, memory)
	created := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "hermes-secret", map[string]any{"slug": "media-case", "title": "Media Case", "body_markdown": "Body"})
	article := decode(t, created)["article"].(map[string]any)
	articleID := int64(article["id"].(float64))

	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, img); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("article_id", strconv.FormatInt(articleID, 10))
	_ = writer.WriteField("caption", "Evidence image")
	_ = writer.WriteField("alt_text", "Orange rectangle")
	_ = writer.WriteField("source_url", "https://example.org/image")
	_ = writer.WriteField("is_cover", "true")
	part, err := writer.CreateFormFile("file", "evidence.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(imageBytes.Bytes())
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer hermes-secret")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("upload=%d %s", res.Code, res.Body.String())
	}
	media := decode(t, res)["media"].(map[string]any)
	mediaID := int64(media["id"].(float64))
	key := media["object_key"].(string)
	if !strings.HasPrefix(key, "articles/media-case/") || !strings.HasSuffix(key, ".jpg") {
		t.Fatalf("key=%s", key)
	}
	stored, ok := memory.Bytes(key)
	if !ok {
		t.Fatal("object not stored")
	}
	if _, format, err := image.Decode(bytes.NewReader(stored)); err != nil || format != "jpeg" {
		t.Fatalf("format=%s err=%v", format, err)
	}

	got := jsonRequest(t, h, http.MethodGet, "/api/v1/articles/media-case", "hermes-secret", nil)
	article = decode(t, got)["article"].(map[string]any)
	if len(article["media"].([]any)) != 1 {
		t.Fatalf("media missing: %v", article)
	}
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/articles/media-case", nil))
	if !strings.Contains(page.Body.String(), "Orange rectangle") || !strings.Contains(page.Body.String(), "https://media.test/") {
		t.Fatalf("page=%s", page.Body.String())
	}

	forbidden := jsonRequest(t, h, http.MethodDelete, "/api/v1/media/"+itoa(mediaID), "hermes-secret", nil)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("hermes delete=%d", forbidden.Code)
	}
	deleted := jsonRequest(t, h, http.MethodDelete, "/api/v1/media/"+itoa(mediaID), "admin-secret", nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", deleted.Code, deleted.Body.String())
	}
	if _, ok := memory.Bytes(key); ok {
		t.Fatal("object not deleted")
	}
}

func TestMediaRejectsNonImage(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "oddity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	memory := storage.NewMemory("https://media.test")
	h := server.NewWithStorage(testConfigWithMax(1024), db, memory)
	created := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "hermes-secret", map[string]any{"slug": "bad-media", "title": "Bad Media", "body_markdown": "Body"})
	id := int64(decode(t, created)["article"].(map[string]any)["id"].(float64))
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("article_id", strconv.FormatInt(id, 10))
	part, _ := writer.CreateFormFile("file", "payload.txt")
	_, _ = part.Write([]byte("not an image"))
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer hermes-secret")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnprocessableEntity {
		var out any
		_ = json.Unmarshal(res.Body.Bytes(), &out)
		t.Fatalf("status=%d body=%v", res.Code, out)
	}
	if memory.Count() != 0 {
		t.Fatal("invalid upload persisted")
	}
}

func TestDeletingArticleDeletesStoredMedia(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "oddity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	memory := storage.NewMemory("https://media.test")
	h := server.NewWithStorage(testConfigWithMax(1<<20), db, memory)
	created := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "hermes-secret", map[string]any{"slug": "delete-media", "title": "Delete Media", "body_markdown": "Body"})
	id := int64(decode(t, created)["article"].(map[string]any)["id"].(float64))
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, img); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("article_id", strconv.FormatInt(id, 10))
	part, _ := writer.CreateFormFile("file", "image.png")
	_, _ = part.Write(imageBytes.Bytes())
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer hermes-secret")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("upload=%d %s", res.Code, res.Body.String())
	}
	key := decode(t, res)["media"].(map[string]any)["object_key"].(string)
	deleted := jsonRequest(t, h, http.MethodDelete, "/api/v1/articles/"+itoa(id), "admin-secret", nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", deleted.Code, deleted.Body.String())
	}
	if _, ok := memory.Bytes(key); ok {
		t.Fatal("orphaned storage object")
	}
}

func TestMediaRejectsDecompressionBombDimensions(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "oddity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	memory := storage.NewMemory("https://media.test")
	h := server.NewWithStorage(testConfigWithMax(1<<20), db, memory)
	created := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "hermes-secret", map[string]any{"slug": "bomb", "title": "Bomb", "body_markdown": "Body"})
	id := int64(decode(t, created)["article"].(map[string]any)["id"].(float64))
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("article_id", strconv.FormatInt(id, 10))
	part, _ := writer.CreateFormFile("file", "bomb.png")
	_, _ = part.Write(pngWithDimensions(50000, 50000))
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer hermes-secret")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	errorBody := decode(t, res)["error"].(map[string]any)
	if errorBody["message"] != "image dimensions too large" {
		t.Fatalf("error=%v", errorBody)
	}
	if memory.Count() != 0 {
		t.Fatal("bomb persisted")
	}
}

func pngWithDimensions(width, height uint32) []byte {
	var out bytes.Buffer
	out.Write([]byte("\x89PNG\r\n\x1a\n"))
	data := make([]byte, 13)
	binary.BigEndian.PutUint32(data[0:4], width)
	binary.BigEndian.PutUint32(data[4:8], height)
	data[8], data[9], data[10], data[11], data[12] = 8, 2, 0, 0, 0
	writePNGChunk(&out, "IHDR", data)
	writePNGChunk(&out, "IEND", nil)
	return out.Bytes()
}

func writePNGChunk(out *bytes.Buffer, kind string, data []byte) {
	_ = binary.Write(out, binary.BigEndian, uint32(len(data)))
	out.WriteString(kind)
	out.Write(data)
	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte(kind))
	_, _ = crc.Write(data)
	_ = binary.Write(out, binary.BigEndian, crc.Sum32())
}

type failingDeleteStorage struct {
	*storage.Memory
	fail bool
}

func (s *failingDeleteStorage) Delete(ctx context.Context, key string) error {
	if s.fail {
		return errors.New("storage unavailable")
	}
	return s.Memory.Delete(ctx, key)
}

func TestArticleDeleteStorageFailureIsRetryable(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "oddity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	memory := storage.NewMemory("https://media.test")
	store := &failingDeleteStorage{Memory: memory, fail: true}
	h := server.NewWithStorage(testConfigWithMax(1<<20), db, store)
	created := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "admin-secret", map[string]any{"slug": "retry-delete", "title": "Retry Delete", "body_markdown": "Body"})
	id := int64(decode(t, created)["article"].(map[string]any)["id"].(float64))
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, img); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("article_id", strconv.FormatInt(id, 10))
	part, _ := writer.CreateFormFile("file", "image.png")
	_, _ = part.Write(imageBytes.Bytes())
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer admin-secret")
	upload := httptest.NewRecorder()
	h.ServeHTTP(upload, req)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload=%d %s", upload.Code, upload.Body.String())
	}
	deleted := jsonRequest(t, h, http.MethodDelete, "/api/v1/articles/"+itoa(id), "admin-secret", nil)
	if deleted.Code != http.StatusBadGateway {
		t.Fatalf("delete=%d %s", deleted.Code, deleted.Body.String())
	}
	stillThere := jsonRequest(t, h, http.MethodGet, "/api/v1/articles/retry-delete", "admin-secret", nil)
	if stillThere.Code != http.StatusNotFound {
		t.Fatalf("article delete was not committed: %d", stillThere.Code)
	}
	var queued int
	if err := db.QueryRow("SELECT COUNT(*) FROM object_deletions WHERE article_id=?", id).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued=%d", queued)
	}
	store.fail = false
	retried := jsonRequest(t, h, http.MethodDelete, "/api/v1/articles/"+itoa(id), "admin-secret", nil)
	if retried.Code != http.StatusNoContent {
		t.Fatalf("retry=%d %s", retried.Code, retried.Body.String())
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM object_deletions WHERE article_id=?", id).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 0 || memory.Count() != 0 {
		t.Fatalf("queue=%d objects=%d", queued, memory.Count())
	}
}
