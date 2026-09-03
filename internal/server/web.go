package server

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/mopga/mythblog/internal/content"
	"github.com/yuin/goldmark"
)

//go:embed templates/*.html static/*
var webFiles embed.FS

var webTemplates = template.Must(template.New("").Funcs(template.FuncMap{
	"articleType":      articleTypeLabel,
	"articleStatus":    articleStatusLabel,
	"credibility":      credibilityLabel,
	"relationLabel":    relationLabel,
	"knownType":        knownType,
	"knownStatus":      knownStatus,
	"knownCredibility": knownCredibility,
}).ParseFS(webFiles, "templates/*.html"))
var markdown = goldmark.New()

type homeData struct {
	Navigation []content.Category
	Articles   []content.Article
}
type articleData struct {
	Navigation []content.Category
	Article    content.Article
	BodyHTML   template.HTML
}
type categoryData struct {
	Navigation []content.Category
	Category   content.Category
	Articles   []content.Article
}

func staticHandler() http.Handler {
	sub, err := fs.Sub(webFiles, "static")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}
func (a *App) home(w http.ResponseWriter, r *http.Request) {
	categories, err := a.content.ListTopCategories(r.Context())
	if err != nil {
		http.Error(w, "internal server error", 500)
		return
	}
	articles, err := a.content.ListPublished(r.Context(), "")
	if err != nil {
		http.Error(w, "internal server error", 500)
		return
	}
	renderHTML(w, "home.html", homeData{categories, articles})
}
func (a *App) publicArticle(w http.ResponseWriter, r *http.Request) {
	article, err := a.content.GetPublishedBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		if err == content.ErrNotFound {
			http.NotFound(w, r)
		} else {
			http.Error(w, "internal server error", 500)
		}
		return
	}
	var rendered bytes.Buffer
	if err = markdown.Convert([]byte(article.BodyMarkdown), &rendered); err != nil {
		http.Error(w, "markdown render failed", 500)
		return
	}
	navigation, err := a.content.ListTopCategories(r.Context())
	if err != nil {
		http.Error(w, "internal server error", 500)
		return
	}
	renderHTML(w, "article.html", articleData{navigation, article, template.HTML(rendered.String())})
}
func (a *App) publicCategory(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	category, err := a.content.GetCategoryBySlug(r.Context(), slug)
	if err != nil {
		if err == content.ErrNotFound {
			http.NotFound(w, r)
		} else {
			http.Error(w, "internal server error", 500)
		}
		return
	}
	articles, err := a.content.ListPublished(r.Context(), slug)
	if err != nil {
		http.Error(w, "internal server error", 500)
		return
	}
	navigation, err := a.content.ListTopCategories(r.Context())
	if err != nil {
		http.Error(w, "internal server error", 500)
		return
	}
	renderHTML(w, "category.html", categoryData{navigation, category, articles})
}

var typeLabels = map[string]string{"story": "История", "case": "Случай", "legend": "Легенда", "creature": "Существо", "conspiracy": "Теория заговора", "government_program": "Государственная программа", "experiment": "Эксперимент", "phenomenon": "Явление", "place": "Место", "person": "Человек", "artifact": "Артефакт", "other": "Материал"}
var statusLabels = map[string]string{"published": "Опубликован", "draft": "Черновик", "archived": "В архиве"}
var credibilityLabels = map[string]string{"confirmed": "Подтверждено", "disputed": "Спорно", "legend": "Легенда", "unknown": "Не установлено"}

func articleTypeLabel(value string) string { return labelOr(value, typeLabels, "Другой тип") }
func articleStatusLabel(value string) string {
	return labelOr(value, statusLabels, "Пользовательский статус")
}
func credibilityLabel(value string) string {
	return labelOr(value, credibilityLabels, "Не установлено")
}
func relationLabel(value string) string {
	return labelOr(value, map[string]string{"related": "Связано", "source": "Источник", "continuation": "Продолжение", "contradicts": "Противоречит"}, "Связано")
}
func labelOr(value string, labels map[string]string, fallback string) string {
	if label, ok := labels[value]; ok {
		return label
	}
	return fallback
}
func knownType(value string) bool        { _, ok := typeLabels[value]; return ok }
func knownStatus(value string) bool      { _, ok := statusLabels[value]; return ok }
func knownCredibility(value string) bool { _, ok := credibilityLabels[value]; return ok }
func renderHTML(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := webTemplates.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "template render failed", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}
