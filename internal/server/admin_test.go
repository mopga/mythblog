package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func adminRequest(h http.Handler, method, path string, form url.Values, auth bool) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if token := form.Get("csrf"); token != "" {
			req.AddCookie(&http.Cookie{Name: "oddity_csrf", Value: token})
		}
	}
	if auth {
		req.SetBasicAuth("admin", "admin-secret")
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}
func csrfFrom(t *testing.T, body string) string {
	t.Helper()
	m := regexp.MustCompile(`name="csrf" value="([0-9a-f]+)"`).FindStringSubmatch(body)
	if len(m) != 2 {
		t.Fatalf("csrf missing: %s", body)
	}
	return m[1]
}

func TestAdminArticleCreateEditDelete(t *testing.T) {
	h, db := newContentApp(t)
	defer db.Close()
	unauth := adminRequest(h, http.MethodGet, "/admin/articles", nil, false)
	if unauth.Code != http.StatusSeeOther || unauth.Header().Get("Location") != "/admin/login" {
		t.Fatalf("unauth=%d", unauth.Code)
	}
	category := jsonRequest(t, h, http.MethodPost, "/api/v1/categories", "admin-secret", map[string]any{"slug": "places", "name": "Places"})
	if category.Code != 201 {
		t.Fatal(category.Body.String())
	}
	newPage := adminRequest(h, http.MethodGet, "/admin/articles/new", nil, true)
	if newPage.Code != 200 {
		t.Fatalf("new=%d", newPage.Code)
	}
	csrf := csrfFrom(t, newPage.Body.String())
	create := adminRequest(h, http.MethodPost, "/admin/articles/new", url.Values{
		"csrf": {csrf}, "slug": {"admin-case"}, "title": {"Admin Case"}, "body_markdown": {"# Body"}, "type": {"case"}, "status": {"published"}, "credibility": {"confirmed"},
		"categories": {"places"}, "sources_text": {"https://example.org/source | Example source | Example"},
	}, true)
	if create.Code != http.StatusSeeOther {
		t.Fatalf("create=%d %s", create.Code, create.Body.String())
	}
	public := httptest.NewRecorder()
	h.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/articles/admin-case", nil))
	if public.Code != 200 {
		t.Fatalf("public=%d", public.Code)
	}
	list := adminRequest(h, http.MethodGet, "/admin/articles", nil, true)
	if !strings.Contains(list.Body.String(), "Admin Case") {
		t.Fatal("admin list missing")
	}

	var id int64
	if err := db.QueryRow("SELECT id FROM articles WHERE slug='admin-case'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	editPage := adminRequest(h, http.MethodGet, "/admin/articles/"+itoa(id), nil, true)
	csrf = csrfFrom(t, editPage.Body.String())
	edit := adminRequest(h, http.MethodPost, "/admin/articles/"+itoa(id), url.Values{"csrf": {csrf}, "slug": {"admin-case"}, "title": {"Edited Case"}, "body_markdown": {"Updated"}, "type": {"case"}, "status": {"published"}, "credibility": {"confirmed"}}, true)
	if edit.Code != http.StatusSeeOther {
		t.Fatalf("edit=%d %s", edit.Code, edit.Body.String())
	}
	got := jsonRequest(t, h, http.MethodGet, "/api/v1/articles/admin-case", "admin-secret", nil)
	if decode(t, got)["article"].(map[string]any)["title"] != "Edited Case" {
		t.Fatal("edit not applied")
	}

	deletePage := adminRequest(h, http.MethodGet, "/admin/articles/"+itoa(id), nil, true)
	csrf = csrfFrom(t, deletePage.Body.String())
	deleted := adminRequest(h, http.MethodPost, "/admin/articles/"+itoa(id)+"/delete", url.Values{"csrf": {csrf}}, true)
	if deleted.Code != http.StatusSeeOther {
		t.Fatalf("delete=%d", deleted.Code)
	}
	missing := jsonRequest(t, h, http.MethodGet, "/api/v1/articles/admin-case", "admin-secret", nil)
	if missing.Code != 404 {
		t.Fatalf("still exists=%d", missing.Code)
	}
}

func TestEditorLoginSessionAndLogout(t *testing.T) {
	h, db := newContentApp(t)
	defer db.Close()
	loginPage := adminRequest(h, http.MethodGet, "/admin/login", nil, false)
	if loginPage.Code != http.StatusOK || !strings.Contains(loginPage.Body.String(), "Вход для редактора") {
		t.Fatalf("login page=%d %s", loginPage.Code, loginPage.Body.String())
	}
	csrf := csrfFrom(t, loginPage.Body.String())
	csrfCookie := loginPage.Result().Cookies()[0]
	if csrfCookie.Name != "oddity_csrf" || !csrfCookie.HttpOnly || csrfCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unsafe csrf cookie: %+v", csrfCookie)
	}
	otherPage := adminRequest(h, http.MethodGet, "/admin/login", nil, false)
	otherCSRF := csrfFrom(t, otherPage.Body.String())
	crossReq := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(url.Values{"csrf": {csrf}, "username": {"editor"}, "password": {"editor-secret-long-enough"}}.Encode()))
	crossReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	crossReq.AddCookie(&http.Cookie{Name: "oddity_csrf", Value: otherCSRF})
	cross := httptest.NewRecorder()
	h.ServeHTTP(cross, crossReq)
	if cross.Code != http.StatusForbidden {
		t.Fatalf("cross-session csrf accepted: %d", cross.Code)
	}
	invalid := adminRequest(h, http.MethodPost, "/admin/login", url.Values{"csrf": {csrf}, "username": {"editor"}, "password": {"wrong-password"}}, false)
	if invalid.Code != http.StatusUnauthorized || invalid.Header().Get("Set-Cookie") != "" || !strings.Contains(invalid.Body.String(), "Неверный логин или пароль") {
		t.Fatalf("invalid login=%d cookie=%q", invalid.Code, invalid.Header().Get("Set-Cookie"))
	}
	login := adminRequest(h, http.MethodPost, "/admin/login", url.Values{"csrf": {csrf}, "username": {"editor"}, "password": {"editor-secret-long-enough"}}, false)
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/admin/articles" {
		t.Fatalf("login=%d location=%s body=%s", login.Code, login.Header().Get("Location"), login.Body.String())
	}
	setCookie := login.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "HttpOnly") || !strings.Contains(setCookie, "SameSite=Strict") || !strings.Contains(setCookie, "Path=/admin") {
		t.Fatalf("unsafe session cookie: %s", setCookie)
	}
	cookie := login.Result().Cookies()[0]
	tampered := *cookie
	tampered.Value += "x"
	tamperedReq := httptest.NewRequest(http.MethodGet, "/admin/articles", nil)
	tamperedReq.AddCookie(&tampered)
	tamperedPage := httptest.NewRecorder()
	h.ServeHTTP(tamperedPage, tamperedReq)
	if tamperedPage.Code != http.StatusSeeOther {
		t.Fatalf("tampered session accepted: %d", tamperedPage.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/articles", nil)
	req.AddCookie(cookie)
	page := httptest.NewRecorder()
	h.ServeHTTP(page, req)
	if page.Code != http.StatusOK {
		t.Fatalf("session page=%d", page.Code)
	}
	logoutReq := httptest.NewRequest(http.MethodPost, "/admin/logout", strings.NewReader(url.Values{"csrf": {csrf}}.Encode()))
	logoutReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	logoutReq.AddCookie(cookie)
	logoutReq.AddCookie(&http.Cookie{Name: "oddity_csrf", Value: csrf})
	logout := httptest.NewRecorder()
	h.ServeHTTP(logout, logoutReq)
	if logout.Code != http.StatusSeeOther || logout.Header().Get("Location") != "/admin/login" {
		t.Fatalf("logout=%d location=%s", logout.Code, logout.Header().Get("Location"))
	}
	if !strings.Contains(logout.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("logout did not clear cookie: %s", logout.Header().Get("Set-Cookie"))
	}
}

func TestAdminPreservesCustomClassificationValues(t *testing.T) {
	h, db := newContentApp(t)
	defer db.Close()
	created := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "admin-secret", map[string]any{
		"slug": "custom-values", "title": "Custom", "body_markdown": "Body", "type": "anomaly", "status": "reviewed", "credibility": "probable",
	})
	id := int64(decode(t, created)["article"].(map[string]any)["id"].(float64))
	page := adminRequest(h, http.MethodGet, "/admin/articles/"+itoa(id), nil, true)
	if !strings.Contains(page.Body.String(), `value="anomaly" selected`) || !strings.Contains(page.Body.String(), "Пользовательский тип") {
		t.Fatalf("custom option missing: %s", page.Body.String())
	}
	csrf := csrfFrom(t, page.Body.String())
	updated := adminRequest(h, http.MethodPost, "/admin/articles/"+itoa(id), url.Values{
		"csrf": {csrf}, "slug": {"custom-values"}, "title": {"Custom"}, "body_markdown": {"Body"}, "type": {"anomaly"}, "status": {"reviewed"}, "credibility": {"probable"},
	}, true)
	if updated.Code != http.StatusSeeOther {
		t.Fatalf("update=%d", updated.Code)
	}
	got := jsonRequest(t, h, http.MethodGet, "/api/v1/articles/custom-values", "admin-secret", nil)
	article := decode(t, got)["article"].(map[string]any)
	if article["type"] != "anomaly" || article["status"] != "reviewed" || article["credibility"] != "probable" {
		t.Fatalf("custom values changed: %v", article)
	}
}

func TestAdminPreservesSourceMetadata(t *testing.T) {
	h, db := newContentApp(t)
	defer db.Close()
	created := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "admin-secret", map[string]any{
		"slug": "source-metadata", "title": "Source Metadata", "body_markdown": "Body",
		"sources": []map[string]any{{"url": "https://example.org/report", "title": "Report", "publisher": "Archive", "published_at": "1997-03-14", "source_type": "report"}},
	})
	id := int64(decode(t, created)["article"].(map[string]any)["id"].(float64))
	editPage := adminRequest(h, http.MethodGet, "/admin/articles/"+itoa(id), nil, true)
	csrf := csrfFrom(t, editPage.Body.String())
	updated := adminRequest(h, http.MethodPost, "/admin/articles/"+itoa(id), url.Values{
		"csrf": {csrf}, "slug": {"source-metadata"}, "title": {"Source Metadata Updated"}, "body_markdown": {"Body"},
		"type": {"other"}, "status": {"published"}, "credibility": {"unknown"},
		"sources_text": {"https://example.org/report | Report | Archive | 1997-03-14 | report"},
	}, true)
	if updated.Code != http.StatusSeeOther {
		t.Fatalf("update=%d %s", updated.Code, updated.Body.String())
	}
	got := jsonRequest(t, h, http.MethodGet, "/api/v1/articles/source-metadata", "admin-secret", nil)
	source := decode(t, got)["article"].(map[string]any)["sources"].([]any)[0].(map[string]any)
	if source["published_at"] != "1997-03-14" || source["source_type"] != "report" {
		t.Fatalf("metadata lost: %v", source)
	}
	var sourceCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM sources").Scan(&sourceCount); err != nil {
		t.Fatal(err)
	}
	if sourceCount != 1 {
		t.Fatalf("orphan source rows=%d", sourceCount-1)
	}
}
