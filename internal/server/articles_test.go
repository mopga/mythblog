package server_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mopga/mythblog/internal/config"
	"github.com/mopga/mythblog/internal/database"
	"github.com/mopga/mythblog/internal/server"
)

func testConfig() config.Config {
	return config.Config{AdminAPIKey: "admin-secret", HermesAPIKey: "hermes-secret", EditorUser: "editor", EditorPassword: "editor-secret-long-enough", SessionSecret: "0123456789abcdef0123456789abcdef", AdminCookiePath: "/admin"}
}

func testConfigWithMax(max int64) config.Config {
	cfg := testConfig()
	cfg.MaxUploadBytes = max
	return cfg
}

func newContentApp(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "oddity.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	return server.New(cfg, db), db
}

func jsonRequest(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func decode(t *testing.T, res *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %d: %v body=%s", res.Code, err, res.Body.String())
	}
	return out
}

func TestArticleAggregateCRUDAndFilters(t *testing.T) {
	h, db := newContentApp(t)
	defer db.Close()
	category := jsonRequest(t, h, http.MethodPost, "/api/v1/categories", "admin-secret", map[string]any{"slug": "ufo", "name": "UFO/UAP", "description": "Unidentified phenomena"})
	if category.Code != http.StatusCreated {
		t.Fatalf("category: %d %s", category.Code, category.Body.String())
	}

	createPayload := map[string]any{
		"slug": "phoenix-lights", "title": "Phoenix Lights", "subtitle": "Arizona, 1997",
		"summary": "Mass sighting", "body_markdown": "# Event\nMultiple witnesses.",
		"type": "case", "status": "published", "credibility": "disputed",
		"event_date_text": "13 March 1997", "location_text": "Arizona, USA",
		"categories": []string{"ufo"},
		"sources":    []map[string]any{{"url": "https://example.test/source", "title": "Primary report", "publisher": "Example", "source_type": "report"}},
		"relations":  []map[string]any{},
	}
	unauthorized := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "", createPayload)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized=%d", unauthorized.Code)
	}
	created := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "hermes-secret", createPayload)
	if created.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", created.Code, created.Body.String())
	}
	article := decode(t, created)["article"].(map[string]any)
	id := int64(article["id"].(float64))
	if article["slug"] != "phoenix-lights" {
		t.Fatalf("article=%v", article)
	}
	if len(article["categories"].([]any)) != 1 || len(article["sources"].([]any)) != 1 {
		t.Fatalf("aggregate=%v", article)
	}

	bySlug := jsonRequest(t, h, http.MethodGet, "/api/v1/articles?slug=phoenix-lights", "hermes-secret", nil)
	if bySlug.Code != http.StatusOK {
		t.Fatalf("list=%d", bySlug.Code)
	}
	if len(decode(t, bySlug)["articles"].([]any)) != 1 {
		t.Fatal("slug filter")
	}
	byTitle := jsonRequest(t, h, http.MethodGet, "/api/v1/articles?title=Phoenix", "hermes-secret", nil)
	if len(decode(t, byTitle)["articles"].([]any)) != 1 {
		t.Fatal("title filter")
	}

	createPayload["title"] = "Phoenix Lights Incident"
	updated := jsonRequest(t, h, http.MethodPut, "/api/v1/articles/"+itoa(id), "hermes-secret", createPayload)
	if updated.Code != http.StatusOK {
		t.Fatalf("update=%d %s", updated.Code, updated.Body.String())
	}
	got := jsonRequest(t, h, http.MethodGet, "/api/v1/articles/phoenix-lights", "hermes-secret", nil)
	if decode(t, got)["article"].(map[string]any)["title"] != "Phoenix Lights Incident" {
		t.Fatal("update missing")
	}
	relatedPayload := map[string]any{
		"slug": "arizona-ufo-wave", "title": "Arizona UFO Wave", "body_markdown": "Related case",
		"type": "case", "status": "published",
		"relations": []map[string]any{{"related_article_id": id, "relation": "related"}},
	}
	related := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "hermes-secret", relatedPayload)
	if related.Code != http.StatusCreated {
		t.Fatalf("related create=%d %s", related.Code, related.Body.String())
	}
	if len(decode(t, related)["article"].(map[string]any)["relations"].([]any)) != 1 {
		t.Fatal("relation missing")
	}

	forbidden := jsonRequest(t, h, http.MethodDelete, "/api/v1/articles/"+itoa(id), "hermes-secret", nil)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("hermes delete=%d", forbidden.Code)
	}
	deleted := jsonRequest(t, h, http.MethodDelete, "/api/v1/articles/"+itoa(id), "admin-secret", nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete=%d", deleted.Code)
	}
}

func TestRelationValidationRollsBackCreate(t *testing.T) {
	h, db := newContentApp(t)
	defer db.Close()
	payload := map[string]any{
		"slug": "bad-relation", "title": "Bad Relation", "body_markdown": "text", "type": "story", "status": "published",
		"relations": []map[string]any{{"related_article_id": 99999, "relation": "related"}},
	}
	res := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "hermes-secret", payload)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	errBody := decode(t, res)["error"].(map[string]any)
	if errBody["code"] != "validation_error" {
		t.Fatalf("error=%v", errBody)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM articles WHERE slug='bad-relation'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("transaction did not roll back")
	}
}

func TestArticleReadsRequireBearerToken(t *testing.T) {
	h, db := newContentApp(t)
	defer db.Close()
	for _, path := range []string{"/api/v1/articles", "/api/v1/articles/anything"} {
		res := jsonRequest(t, h, http.MethodGet, path, "", nil)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("path=%s status=%d", path, res.Code)
		}
	}
}

func TestDuplicateSourcesAndRelationsReturnValidationError(t *testing.T) {
	h, db := newContentApp(t)
	defer db.Close()
	base := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "hermes-secret", map[string]any{"slug": "base", "title": "Base", "body_markdown": "base"})
	baseID := int64(decode(t, base)["article"].(map[string]any)["id"].(float64))
	tests := []map[string]any{
		{"slug": "duplicate-sources", "title": "Duplicate sources", "body_markdown": "body", "sources": []map[string]any{{"url": "https://example.test/a"}, {"url": "https://example.test/a"}}},
		{"slug": "duplicate-relations", "title": "Duplicate relations", "body_markdown": "body", "relations": []map[string]any{{"related_article_id": baseID, "relation": "related"}, {"related_article_id": baseID, "relation": "related"}}},
	}
	for _, payload := range tests {
		res := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "hermes-secret", payload)
		if res.Code != http.StatusUnprocessableEntity {
			t.Fatalf("slug=%s status=%d body=%s", payload["slug"], res.Code, res.Body.String())
		}
	}
}

func TestSourceMetadataIsScopedPerArticle(t *testing.T) {
	h, db := newContentApp(t)
	defer db.Close()
	for _, payload := range []map[string]any{
		{"slug": "source-a", "title": "A", "body_markdown": "A", "sources": []map[string]any{{"url": "https://example.test/shared", "title": "Title A", "publisher": "Publisher A"}}},
		{"slug": "source-b", "title": "B", "body_markdown": "B", "sources": []map[string]any{{"url": "https://example.test/shared", "title": "Title B", "publisher": "Publisher B"}}},
	} {
		res := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "admin-secret", payload)
		if res.Code != http.StatusCreated {
			t.Fatalf("create=%d %s", res.Code, res.Body.String())
		}
	}
	first := jsonRequest(t, h, http.MethodGet, "/api/v1/articles/source-a", "admin-secret", nil)
	source := decode(t, first)["article"].(map[string]any)["sources"].([]any)[0].(map[string]any)
	if source["title"] != "Title A" || source["publisher"] != "Publisher A" {
		t.Fatalf("metadata mutated across articles: %v", source)
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
