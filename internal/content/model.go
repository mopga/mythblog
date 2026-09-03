package content

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	slugRE      = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	valueRE     = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
)

type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "validation failed" }

type Category struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ParentID    *int64 `json:"parent_id,omitempty"`
	CreatedAt   string `json:"created_at"`
}
type CategoryInput struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ParentID    *int64 `json:"parent_id"`
}
type Source struct {
	ID          int64   `json:"id"`
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Publisher   string  `json:"publisher"`
	PublishedAt *string `json:"published_at,omitempty"`
	SourceType  string  `json:"source_type"`
	CreatedAt   string  `json:"created_at"`
}
type SourceInput struct {
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Publisher   string  `json:"publisher"`
	PublishedAt *string `json:"published_at"`
	SourceType  string  `json:"source_type"`
}
type Relation struct {
	RelatedArticleID int64  `json:"related_article_id"`
	Relation         string `json:"relation"`
	Slug             string `json:"slug,omitempty"`
	Title            string `json:"title,omitempty"`
}
type RelationInput struct {
	RelatedArticleID int64  `json:"related_article_id"`
	Relation         string `json:"relation"`
}
type Media struct {
	ID        int64  `json:"id"`
	ArticleID int64  `json:"article_id"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	ObjectKey string `json:"object_key"`
	SourceURL string `json:"source_url"`
	Caption   string `json:"caption"`
	AltText   string `json:"alt_text"`
	License   string `json:"license"`
	IsCover   bool   `json:"is_cover"`
	SortOrder int    `json:"sort_order"`
	CreatedAt string `json:"created_at"`
}
type MediaInput struct {
	ArticleID                                                  int64
	Type, URL, ObjectKey, SourceURL, Caption, AltText, License string
	IsCover                                                    bool
	SortOrder                                                  int
}
type ObjectDeletion struct {
	ID        int64
	ArticleID int64
	ObjectKey string
}

type Article struct {
	ID            int64      `json:"id"`
	Slug          string     `json:"slug"`
	Title         string     `json:"title"`
	Subtitle      string     `json:"subtitle"`
	Summary       string     `json:"summary"`
	BodyMarkdown  string     `json:"body_markdown"`
	Type          string     `json:"type"`
	Status        string     `json:"status"`
	Credibility   string     `json:"credibility"`
	EventDateText string     `json:"event_date_text"`
	LocationText  string     `json:"location_text"`
	CreatedAt     string     `json:"created_at"`
	UpdatedAt     string     `json:"updated_at"`
	Categories    []Category `json:"categories"`
	Sources       []Source   `json:"sources"`
	Relations     []Relation `json:"relations"`
	Media         []Media    `json:"media"`
}
type ArticleInput struct {
	Slug          string          `json:"slug"`
	Title         string          `json:"title"`
	Subtitle      string          `json:"subtitle"`
	Summary       string          `json:"summary"`
	BodyMarkdown  string          `json:"body_markdown"`
	Type          string          `json:"type"`
	Status        string          `json:"status"`
	Credibility   string          `json:"credibility"`
	EventDateText string          `json:"event_date_text"`
	LocationText  string          `json:"location_text"`
	Categories    []string        `json:"categories"`
	Sources       []SourceInput   `json:"sources"`
	Relations     []RelationInput `json:"relations"`
}

func (in *ArticleInput) Normalize() {
	in.Slug = strings.TrimSpace(strings.ToLower(in.Slug))
	in.Title = strings.TrimSpace(in.Title)
	in.Type = strings.TrimSpace(in.Type)
	in.Status = strings.TrimSpace(in.Status)
	in.Credibility = strings.TrimSpace(in.Credibility)
	if in.Type == "" {
		in.Type = "other"
	}
	if in.Status == "" {
		in.Status = "published"
	}
	if in.Credibility == "" {
		in.Credibility = "unknown"
	}
	if in.Categories == nil {
		in.Categories = []string{}
	}
	if in.Sources == nil {
		in.Sources = []SourceInput{}
	}
	if in.Relations == nil {
		in.Relations = []RelationInput{}
	}
}
func ValidateArticle(in ArticleInput) error {
	fields := map[string]string{}
	if !slugRE.MatchString(in.Slug) || len(in.Slug) > 160 {
		fields["slug"] = "must be a lowercase kebab-case slug up to 160 characters"
	}
	if in.Title == "" || len(in.Title) > 300 {
		fields["title"] = "required, maximum 300 characters"
	}
	if strings.TrimSpace(in.BodyMarkdown) == "" {
		fields["body_markdown"] = "required"
	}
	if !valueRE.MatchString(in.Type) {
		fields["type"] = "invalid"
	}
	if !valueRE.MatchString(in.Status) {
		fields["status"] = "invalid"
	}
	if !valueRE.MatchString(in.Credibility) {
		fields["credibility"] = "invalid"
	}
	seen := map[string]bool{}
	for _, slug := range in.Categories {
		if !slugRE.MatchString(slug) {
			fields["categories"] = "contains invalid slug"
		}
		if seen[slug] {
			fields["categories"] = "contains duplicates"
		}
		seen[slug] = true
	}
	for i, s := range in.Sources {
		u, err := url.ParseRequestURI(s.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			fields[fmt.Sprintf("sources.%d.url", i)] = "must be an http(s) URL"
		}
	}
	for i, r := range in.Relations {
		if r.RelatedArticleID < 1 || !valueRE.MatchString(r.Relation) {
			fields[fmt.Sprintf("relations.%d", i)] = "invalid"
		}
	}
	seenSource := map[string]bool{}
	for i, s := range in.Sources {
		if seenSource[s.URL] {
			fields[fmt.Sprintf("sources.%d.url", i)] = "duplicate URL"
		}
		seenSource[s.URL] = true
	}
	seenRelation := map[string]bool{}
	for i, r := range in.Relations {
		key := fmt.Sprintf("%d:%s", r.RelatedArticleID, r.Relation)
		if seenRelation[key] {
			fields[fmt.Sprintf("relations.%d", i)] = "duplicate relation"
		}
		seenRelation[key] = true
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}
func ValidateCategory(in CategoryInput) error {
	fields := map[string]string{}
	if !slugRE.MatchString(in.Slug) {
		fields["slug"] = "invalid"
	}
	if strings.TrimSpace(in.Name) == "" {
		fields["name"] = "required"
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}
