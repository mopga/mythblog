package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mopga/mythblog/internal/config"
	"github.com/mopga/mythblog/internal/content"
	"github.com/mopga/mythblog/internal/storage"
)

type App struct {
	cfg     config.Config
	db      *sql.DB
	router  chi.Router
	content *content.Repository
	storage storage.ObjectStorage
	mediaMu sync.Mutex
}

func New(cfg config.Config, db *sql.DB) http.Handler {
	store, err := storage.FromConfig(cfg)
	if err != nil {
		panic(err)
	}
	return NewWithStorage(cfg, db, store)
}
func NewWithStorage(cfg config.Config, db *sql.DB, store storage.ObjectStorage) http.Handler {
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	app := &App{cfg: cfg, db: db, content: content.NewRepository(db), storage: store}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, app.logRequest)
	r.Get("/health", app.health)
	r.Get("/", app.home)
	r.Get("/articles/{slug}", app.publicArticle)
	r.Get("/categories/{slug}", app.publicCategory)
	r.Handle("/static/*", staticHandler())
	if local, ok := store.(*storage.Local); ok {
		r.Handle("/media/*", http.StripPrefix("/media/", http.FileServer(http.Dir(local.Root()))))
	}
	r.Route("/admin", func(r chi.Router) {
		r.Get("/login", app.adminLoginPage)
		r.Post("/login", app.adminLogin)
		r.Group(func(r chi.Router) {
			r.Use(app.requireAdminSession)
			r.Get("/", app.adminIndex)
			r.Get("/articles", app.adminArticles)
			r.Get("/articles/new", app.adminNew)
			r.Post("/articles/new", app.adminCreate)
			r.Get("/articles/{id}", app.adminEdit)
			r.Post("/articles/{id}", app.adminUpdate)
			r.Post("/articles/{id}/media", app.adminUploadMedia)
			r.Post("/articles/{id}/media/{mediaID}/delete", app.adminDeleteMedia)
			r.Post("/articles/{id}/delete", app.adminDelete)
			r.Post("/logout", app.adminLogout)
		})
	})
	r.Route("/api/v1", func(r chi.Router) {
		r.With(app.requireReader).Get("/articles", app.listArticles)
		r.With(app.requireReader).Get("/articles/{key}", app.getArticle)
		r.With(app.requireWriter).Post("/articles", app.createArticle)
		r.With(app.requireWriter).Put("/articles/{id}", app.updateArticle)
		r.With(app.requireAdmin).Delete("/articles/{id}", app.deleteArticle)
		r.Get("/categories", app.listCategories)
		r.With(app.requireWriter).Post("/categories", app.createCategory)
		r.With(app.requireWriter).Post("/media", app.uploadMedia)
		r.With(app.requireAdmin).Delete("/media/{id}", app.deleteMedia)
	})
	app.router = r
	return r
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.PingContext(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "degraded", "database": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "database": "ok"})
}

func (a *App) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "duration_ms", time.Since(start).Milliseconds(), "request_id", middleware.GetReqID(r.Context()))
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
