package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicSSRFromAPIContentAndSafeMarkdown(t *testing.T) {
	h, db := newContentApp(t)
	defer db.Close()
	category := jsonRequest(t, h, http.MethodPost, "/api/v1/categories", "admin-secret", map[string]any{"slug": "legends", "name": "Legends"})
	if category.Code != 201 {
		t.Fatal(category.Body.String())
	}
	article := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "hermes-secret", map[string]any{
		"slug": "vanishing-hitchhiker", "title": "Vanishing Hitchhiker", "subtitle": "A recurring road legend",
		"summary":       "A traveler disappears from a moving car.",
		"body_markdown": "# Account\n**Witnesses** report it.\n<script>alert('xss')</script>",
		"type":          "legend", "status": "published", "credibility": "legend", "categories": []string{"legends"},
		"sources": []map[string]any{{"url": "https://example.org/legend", "title": "Folklore index"}},
	})
	if article.Code != 201 {
		t.Fatal(article.Body.String())
	}
	draft := jsonRequest(t, h, http.MethodPost, "/api/v1/articles", "admin-secret", map[string]any{"slug": "hidden-draft", "title": "Hidden Draft", "body_markdown": "secret", "status": "draft"})
	if draft.Code != 201 {
		t.Fatal(draft.Body.String())
	}

	home := httptest.NewRecorder()
	h.ServeHTTP(home, httptest.NewRequest(http.MethodGet, "/", nil))
	if home.Code != 200 {
		t.Fatalf("home=%d %s", home.Code, home.Body.String())
	}
	if !strings.Contains(home.Body.String(), "Vanishing Hitchhiker") || strings.Contains(home.Body.String(), "Hidden Draft") {
		t.Fatalf("home body=%s", home.Body.String())
	}
	if !strings.Contains(home.Body.String(), `lang="ru"`) || !strings.Contains(home.Body.String(), "Последние материалы") {
		t.Fatalf("home is not localized: %s", home.Body.String())
	}
	if !strings.Contains(home.Body.String(), `class="category-nav"`) || !strings.Contains(home.Body.String(), `/categories/legends`) {
		t.Fatalf("category navigation missing: %s", home.Body.String())
	}

	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/articles/vanishing-hitchhiker", nil))
	body := page.Body.String()
	if page.Code != 200 || !strings.Contains(body, "<h1>Account</h1>") || !strings.Contains(body, "<strong>Witnesses</strong>") {
		t.Fatalf("article=%d %s", page.Code, body)
	}
	if strings.Contains(strings.ToLower(body), "<script") || strings.Contains(body, "alert('xss')") {
		t.Fatalf("unsafe markdown=%s", body)
	}
	if !strings.Contains(body, "https://example.org/legend") {
		t.Fatal("source missing")
	}
	if !strings.Contains(body, "Источники") || !strings.Contains(body, `class="category-nav"`) {
		t.Fatalf("article shell is not Russian/navigation missing: %s", body)
	}

	categoryPage := httptest.NewRecorder()
	h.ServeHTTP(categoryPage, httptest.NewRequest(http.MethodGet, "/categories/legends", nil))
	if categoryPage.Code != 200 || !strings.Contains(categoryPage.Body.String(), "Vanishing Hitchhiker") {
		t.Fatalf("category=%d %s", categoryPage.Code, categoryPage.Body.String())
	}
}
