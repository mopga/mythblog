package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mopga/mythblog/internal/content"
)

type categoryOption struct {
	Category content.Category
	Checked  bool
}
type adminFormData struct {
	ID            int64
	IsNew         bool
	CSRF          string
	Error         string
	Input         content.ArticleInput
	Categories    []categoryOption
	SourcesText   string
	RelationsText string
	Media         []content.Media
}

type adminListData struct {
	Articles []content.Article
	CSRF     string
}

func (a *App) requireAdminBasic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "admin" || !secureEqual(password, a.cfg.AdminAPIKey) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Oddity Admin", charset="UTF-8"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *App) ensureCSRFToken(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(csrfCookieName); err == nil && len(cookie.Value) == 64 {
		return cookie.Value
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	token := hex.EncodeToString(raw)
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: token, Path: a.adminCookiePath(), MaxAge: int(editorSessionTTL.Seconds()), HttpOnly: true, Secure: a.cfg.SecureCookies, SameSite: http.SameSiteStrictMode})
	return token
}
func (a *App) validCSRF(r *http.Request, value string) bool {
	cookie, err := r.Cookie(csrfCookieName)
	return err == nil && secureEqual(value, cookie.Value)
}

func (a *App) adminIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
}
func (a *App) adminArticles(w http.ResponseWriter, r *http.Request) {
	articles, err := a.content.List(r.Context(), "", "")
	if err != nil {
		http.Error(w, "internal server error", 500)
		return
	}
	renderHTML(w, "admin_articles.html", adminListData{Articles: articles, CSRF: a.ensureCSRFToken(w, r)})
}
func (a *App) adminNew(w http.ResponseWriter, r *http.Request) {
	a.renderAdminForm(w, r, adminFormData{IsNew: true, Input: content.ArticleInput{Type: "other", Status: "published", Credibility: "unknown"}})
}
func (a *App) adminCreate(w http.ResponseWriter, r *http.Request) {
	input, sources, relations, err := a.parseAdminForm(w, r)
	data := adminFormData{IsNew: true, Input: input, SourcesText: sources, RelationsText: relations}
	if err != nil {
		data.Error = adminErrorMessage(err)
		a.renderAdminFormStatus(w, r, data, 422)
		return
	}
	if _, err = a.content.Create(r.Context(), input); err != nil {
		data.Error = adminErrorMessage(err)
		a.renderAdminFormStatus(w, r, data, 422)
		return
	}
	http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
}
func (a *App) adminEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := adminID(w, r)
	if !ok {
		return
	}
	article, err := a.content.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	input := inputFromArticle(article)
	a.renderAdminForm(w, r, adminFormData{ID: id, Input: input, SourcesText: sourcesText(article.Sources), RelationsText: relationsText(article.Relations), Media: article.Media})
}
func (a *App) adminUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := adminID(w, r)
	if !ok {
		return
	}
	input, sources, relations, err := a.parseAdminForm(w, r)
	data := adminFormData{ID: id, Input: input, SourcesText: sources, RelationsText: relations}
	if err != nil {
		data.Error = adminErrorMessage(err)
		a.renderAdminFormStatus(w, r, data, 422)
		return
	}
	if _, err = a.content.Update(r.Context(), id, input); err != nil {
		data.Error = adminErrorMessage(err)
		a.renderAdminFormStatus(w, r, data, 422)
		return
	}
	http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
}
func (a *App) adminDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := adminID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil || !a.validCSRF(r, r.FormValue("csrf")) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	if err := a.deleteArticleWithMedia(r, id); err != nil {
		if errors.Is(err, errStorageDelete) {
			http.Error(w, "object storage delete failed", http.StatusBadGateway)
			return
		}
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
}

func (a *App) adminUploadMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := adminID(w, r)
	if !ok {
		return
	}
	limit := a.cfg.MaxUploadBytes
	if limit < 1 {
		limit = 10 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
	if err := r.ParseMultipartForm(limit); err != nil {
		http.Error(w, "invalid upload", 422)
		return
	}
	if !a.validCSRF(r, r.FormValue("csrf")) {
		http.Error(w, "invalid CSRF token", 403)
		return
	}
	r.Form.Set("article_id", strconv.FormatInt(id, 10))
	if _, failure := a.createMedia(w, r); failure != nil {
		http.Error(w, failure.message, failure.status)
		return
	}
	http.Redirect(w, r, "/admin/articles/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}
func (a *App) adminDeleteMedia(w http.ResponseWriter, r *http.Request) {
	articleID, ok := adminID(w, r)
	if !ok {
		return
	}
	mediaID, err := strconv.ParseInt(chi.URLParam(r, "mediaID"), 10, 64)
	if err != nil || mediaID < 1 {
		http.Error(w, "invalid media id", 400)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err = r.ParseForm(); err != nil || !a.validCSRF(r, r.FormValue("csrf")) {
		http.Error(w, "invalid CSRF token", 403)
		return
	}
	media, err := a.content.GetMedia(r.Context(), mediaID)
	if err != nil || media.ArticleID != articleID {
		http.NotFound(w, r)
		return
	}
	if failure := a.removeMedia(r, mediaID); failure != nil {
		http.Error(w, failure.message, failure.status)
		return
	}
	http.Redirect(w, r, "/admin/articles/"+strconv.FormatInt(articleID, 10), http.StatusSeeOther)
}
func adminID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		http.Error(w, "invalid id", 400)
		return 0, false
	}
	return id, true
}

func (a *App) parseAdminForm(w http.ResponseWriter, r *http.Request) (content.ArticleInput, string, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	if err := r.ParseForm(); err != nil {
		return content.ArticleInput{}, "", "", err
	}
	sourcesRaw := r.FormValue("sources_text")
	relationsRaw := r.FormValue("relations_text")
	if !a.validCSRF(r, r.FormValue("csrf")) {
		return content.ArticleInput{}, sourcesRaw, relationsRaw, &content.ValidationError{Fields: map[string]string{"csrf": "invalid token"}}
	}
	input := content.ArticleInput{Slug: r.FormValue("slug"), Title: r.FormValue("title"), Subtitle: r.FormValue("subtitle"), Summary: r.FormValue("summary"), BodyMarkdown: r.FormValue("body_markdown"), Type: r.FormValue("type"), Status: r.FormValue("status"), Credibility: r.FormValue("credibility"), EventDateText: r.FormValue("event_date_text"), LocationText: r.FormValue("location_text"), Categories: r.Form["categories"], Sources: []content.SourceInput{}, Relations: []content.RelationInput{}}
	for _, line := range strings.Split(sourcesRaw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		source := content.SourceInput{URL: strings.TrimSpace(parts[0])}
		if len(parts) > 1 {
			source.Title = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			source.Publisher = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 && strings.TrimSpace(parts[3]) != "" {
			publishedAt := strings.TrimSpace(parts[3])
			source.PublishedAt = &publishedAt
		}
		if len(parts) > 4 {
			source.SourceType = strings.TrimSpace(parts[4])
		}
		input.Sources = append(input.Sources, source)
	}
	for _, line := range strings.Split(relationsRaw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		id, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil || len(parts) != 2 {
			return input, sourcesRaw, relationsRaw, &content.ValidationError{Fields: map[string]string{"relations": "use ID | relation"}}
		}
		input.Relations = append(input.Relations, content.RelationInput{RelatedArticleID: id, Relation: strings.TrimSpace(parts[1])})
	}
	return input, sourcesRaw, relationsRaw, nil
}
func (a *App) renderAdminForm(w http.ResponseWriter, r *http.Request, data adminFormData) {
	a.renderAdminFormStatus(w, r, data, 200)
}
func (a *App) renderAdminFormStatus(w http.ResponseWriter, r *http.Request, data adminFormData, status int) {
	categories, err := a.content.ListCategories(r.Context())
	if err != nil {
		http.Error(w, "internal server error", 500)
		return
	}
	selected := map[string]bool{}
	for _, slug := range data.Input.Categories {
		selected[slug] = true
	}
	data.Categories = make([]categoryOption, 0, len(categories))
	for _, category := range categories {
		data.Categories = append(data.Categories, categoryOption{category, selected[category.Slug]})
	}
	data.CSRF = a.ensureCSRFToken(w, r)
	var buf bytes.Buffer
	if err := webTemplates.ExecuteTemplate(&buf, "admin_form.html", data); err != nil {
		http.Error(w, "template render failed", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}
func inputFromArticle(a content.Article) content.ArticleInput {
	input := content.ArticleInput{Slug: a.Slug, Title: a.Title, Subtitle: a.Subtitle, Summary: a.Summary, BodyMarkdown: a.BodyMarkdown, Type: a.Type, Status: a.Status, Credibility: a.Credibility, EventDateText: a.EventDateText, LocationText: a.LocationText, Categories: []string{}, Sources: []content.SourceInput{}, Relations: []content.RelationInput{}}
	for _, c := range a.Categories {
		input.Categories = append(input.Categories, c.Slug)
	}
	for _, s := range a.Sources {
		input.Sources = append(input.Sources, content.SourceInput{URL: s.URL, Title: s.Title, Publisher: s.Publisher, PublishedAt: s.PublishedAt, SourceType: s.SourceType})
	}
	for _, rel := range a.Relations {
		input.Relations = append(input.Relations, content.RelationInput{RelatedArticleID: rel.RelatedArticleID, Relation: rel.Relation})
	}
	return input
}
func adminErrorMessage(err error) string {
	var validation *content.ValidationError
	switch {
	case errors.As(err, &validation):
		return "Проверьте обязательные поля и формат значений"
	case errors.Is(err, content.ErrConflict):
		return "Материал с таким адресом уже существует"
	default:
		return "Не удалось сохранить материал"
	}
}

func sourcesText(sources []content.Source) string {
	lines := []string{}
	for _, s := range sources {
		publishedAt := ""
		if s.PublishedAt != nil {
			publishedAt = *s.PublishedAt
		}
		lines = append(lines, s.URL+" | "+s.Title+" | "+s.Publisher+" | "+publishedAt+" | "+s.SourceType)
	}
	return strings.Join(lines, "\n")
}
func relationsText(relations []content.Relation) string {
	lines := []string{}
	for _, r := range relations {
		lines = append(lines, strconv.FormatInt(r.RelatedArticleID, 10)+" | "+r.Relation)
	}
	return strings.Join(lines, "\n")
}
