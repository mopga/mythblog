package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mopga/mythblog/internal/content"
)

func (a *App) requireWriter(next http.Handler) http.Handler { return a.requireRole(false, next) }
func (a *App) requireAdmin(next http.Handler) http.Handler  { return a.requireRole(true, next) }
func (a *App) requireReader(next http.Handler) http.Handler { return a.requireRole(false, next) }
func (a *App) requireRole(adminOnly bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "Bearer token required", nil)
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		admin := secureEqual(token, a.cfg.AdminAPIKey)
		hermes := secureEqual(token, a.cfg.HermesAPIKey)
		if adminOnly && !admin {
			if hermes {
				writeAPIError(w, http.StatusForbidden, "forbidden", "admin token required", nil)
			} else {
				writeAPIError(w, http.StatusUnauthorized, "unauthorized", "invalid token", nil)
			}
			return
		}
		if !adminOnly && !admin && !hermes {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "invalid token", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func secureEqual(a, b string) bool {
	if b == "" {
		return false
	}
	aHash := sha256.Sum256([]byte(a))
	bHash := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(aHash[:], bHash[:]) == 1
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON object")
	}
	return nil
}
func writeAPIError(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "fields": fields}})
}
func handleContentError(w http.ResponseWriter, err error) {
	var validation *content.ValidationError
	switch {
	case errors.As(err, &validation):
		writeAPIError(w, http.StatusUnprocessableEntity, "validation_error", err.Error(), validation.Fields)
	case errors.Is(err, content.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "resource not found", nil)
	case errors.Is(err, content.ErrConflict):
		writeAPIError(w, http.StatusConflict, "conflict", "resource already exists", nil)
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error", nil)
	}
}

func (a *App) listArticles(w http.ResponseWriter, r *http.Request) {
	articles, err := a.content.List(r.Context(), r.URL.Query().Get("slug"), r.URL.Query().Get("title"))
	if err != nil {
		handleContentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"articles": articles})
}
func (a *App) getArticle(w http.ResponseWriter, r *http.Request) {
	article, err := a.content.GetBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		handleContentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"article": article})
}
func (a *App) createArticle(w http.ResponseWriter, r *http.Request) {
	var input content.ArticleInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	article, err := a.content.Create(r.Context(), input)
	if err != nil {
		handleContentError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"article": article})
}
func (a *App) updateArticle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeAPIError(w, http.StatusBadRequest, "invalid_id", "article id is invalid", nil)
		return
	}
	var input content.ArticleInput
	if err = decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	article, err := a.content.Update(r.Context(), id, input)
	if err != nil {
		handleContentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"article": article})
}
func (a *App) deleteArticle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeAPIError(w, http.StatusBadRequest, "invalid_id", "article id is invalid", nil)
		return
	}
	if err = a.deleteArticleWithMedia(r, id); err != nil {
		if errors.Is(err, errStorageDelete) {
			writeAPIError(w, http.StatusBadGateway, "storage_error", "object storage delete failed", nil)
			return
		}
		handleContentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *App) listCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := a.content.ListCategories(r.Context())
	if err != nil {
		handleContentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": categories})
}
func (a *App) createCategory(w http.ResponseWriter, r *http.Request) {
	var input content.CategoryInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	category, err := a.content.CreateCategory(r.Context(), input)
	if err != nil {
		handleContentError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"category": category})
}
