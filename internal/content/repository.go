package content

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

type queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *Repository) Create(ctx context.Context, in ArticleInput) (Article, error) {
	in.Normalize()
	if err := ValidateArticle(in); err != nil {
		return Article{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Article{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `INSERT INTO articles(slug,title,subtitle,summary,body_markdown,type,status,credibility,event_date_text,location_text,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, in.Slug, in.Title, in.Subtitle, in.Summary, in.BodyMarkdown, in.Type, in.Status, in.Credibility, in.EventDateText, in.LocationText, now, now)
	if unique(err) {
		return Article{}, ErrConflict
	}
	if err != nil {
		return Article{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Article{}, err
	}
	if err = replaceLinks(ctx, tx, id, in); err != nil {
		return Article{}, err
	}
	if err = tx.Commit(); err != nil {
		return Article{}, err
	}
	return r.GetByID(ctx, id)
}
func (r *Repository) Update(ctx context.Context, id int64, in ArticleInput) (Article, error) {
	in.Normalize()
	if err := ValidateArticle(in); err != nil {
		return Article{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Article{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE articles SET slug=?,title=?,subtitle=?,summary=?,body_markdown=?,type=?,status=?,credibility=?,event_date_text=?,location_text=?,updated_at=? WHERE id=?`, in.Slug, in.Title, in.Subtitle, in.Summary, in.BodyMarkdown, in.Type, in.Status, in.Credibility, in.EventDateText, in.LocationText, now, id)
	if unique(err) {
		return Article{}, ErrConflict
	}
	if err != nil {
		return Article{}, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return Article{}, ErrNotFound
	}
	for _, table := range []string{"article_categories", "article_sources", "article_relations"} {
		if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE article_id=?", id); err != nil {
			return Article{}, err
		}
	}
	if err = replaceLinks(ctx, tx, id, in); err != nil {
		return Article{}, err
	}
	if err = tx.Commit(); err != nil {
		return Article{}, err
	}
	return r.GetByID(ctx, id)
}
func replaceLinks(ctx context.Context, q queryer, articleID int64, in ArticleInput) error {
	for _, slug := range in.Categories {
		var id int64
		if err := q.QueryRowContext(ctx, "SELECT id FROM categories WHERE slug=?", slug).Scan(&id); errors.Is(err, sql.ErrNoRows) {
			return &ValidationError{Fields: map[string]string{"categories": "category does not exist: " + slug}}
		} else if err != nil {
			return err
		}
		if _, err := q.ExecContext(ctx, "INSERT INTO article_categories(article_id,category_id) VALUES(?,?)", articleID, id); err != nil {
			return err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, source := range in.Sources {
		result, err := q.ExecContext(ctx, `INSERT INTO sources(url,title,publisher,published_at,source_type,created_at) VALUES(?,?,?,?,?,?)`, source.URL, source.Title, source.Publisher, source.PublishedAt, source.SourceType, now)
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		if _, err = q.ExecContext(ctx, "INSERT INTO article_sources(article_id,source_id) VALUES(?,?)", articleID, id); err != nil {
			return err
		}
	}
	for _, relation := range in.Relations {
		if relation.RelatedArticleID == articleID {
			return &ValidationError{Fields: map[string]string{"relations": "self relation is not allowed"}}
		}
		var exists int
		if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM articles WHERE id=?", relation.RelatedArticleID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return &ValidationError{Fields: map[string]string{"relations": fmt.Sprintf("related article %d does not exist", relation.RelatedArticleID)}}
		}
		if _, err := q.ExecContext(ctx, "INSERT INTO article_relations(article_id,related_article_id,relation) VALUES(?,?,?)", articleID, relation.RelatedArticleID, relation.Relation); err != nil {
			return err
		}
	}
	return nil
}
func (r *Repository) GetBySlug(ctx context.Context, slug string) (Article, error) {
	return r.get(ctx, "slug=?", slug)
}
func (r *Repository) GetByID(ctx context.Context, id int64) (Article, error) {
	return r.get(ctx, "id=?", id)
}
func (r *Repository) get(ctx context.Context, where string, arg any) (Article, error) {
	var a Article
	err := r.db.QueryRowContext(ctx, `SELECT id,slug,title,subtitle,summary,body_markdown,type,status,credibility,event_date_text,location_text,created_at,updated_at FROM articles WHERE `+where, arg).Scan(&a.ID, &a.Slug, &a.Title, &a.Subtitle, &a.Summary, &a.BodyMarkdown, &a.Type, &a.Status, &a.Credibility, &a.EventDateText, &a.LocationText, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Article{}, ErrNotFound
	}
	if err != nil {
		return Article{}, err
	}
	a.Categories = []Category{}
	rows, err := r.db.QueryContext(ctx, `SELECT c.id,c.slug,c.name,c.description,c.parent_id,c.created_at FROM categories c JOIN article_categories ac ON ac.category_id=c.id WHERE ac.article_id=? ORDER BY c.name`, a.ID)
	if err != nil {
		return Article{}, err
	}
	for rows.Next() {
		var c Category
		if err = rows.Scan(&c.ID, &c.Slug, &c.Name, &c.Description, &c.ParentID, &c.CreatedAt); err != nil {
			rows.Close()
			return Article{}, err
		}
		a.Categories = append(a.Categories, c)
	}
	rows.Close()
	a.Sources = []Source{}
	rows, err = r.db.QueryContext(ctx, `SELECT s.id,s.url,s.title,s.publisher,s.published_at,s.source_type,s.created_at FROM sources s JOIN article_sources x ON x.source_id=s.id WHERE x.article_id=? ORDER BY s.id`, a.ID)
	if err != nil {
		return Article{}, err
	}
	for rows.Next() {
		var s Source
		if err = rows.Scan(&s.ID, &s.URL, &s.Title, &s.Publisher, &s.PublishedAt, &s.SourceType, &s.CreatedAt); err != nil {
			rows.Close()
			return Article{}, err
		}
		a.Sources = append(a.Sources, s)
	}
	rows.Close()
	a.Relations = []Relation{}
	rows, err = r.db.QueryContext(ctx, `SELECT ar.related_article_id,ar.relation,a.slug,a.title FROM article_relations ar JOIN articles a ON a.id=ar.related_article_id WHERE ar.article_id=? ORDER BY a.title`, a.ID)
	if err != nil {
		return Article{}, err
	}
	for rows.Next() {
		var rel Relation
		if err = rows.Scan(&rel.RelatedArticleID, &rel.Relation, &rel.Slug, &rel.Title); err != nil {
			rows.Close()
			return Article{}, err
		}
		a.Relations = append(a.Relations, rel)
	}
	rows.Close()
	a.Media = []Media{}
	rows, err = r.db.QueryContext(ctx, `SELECT id,article_id,type,url,object_key,source_url,caption,alt_text,license,is_cover,sort_order,created_at FROM media WHERE article_id=? ORDER BY is_cover DESC,sort_order,id`, a.ID)
	if err != nil {
		return Article{}, err
	}
	for rows.Next() {
		var media Media
		var cover int
		if err = rows.Scan(&media.ID, &media.ArticleID, &media.Type, &media.URL, &media.ObjectKey, &media.SourceURL, &media.Caption, &media.AltText, &media.License, &cover, &media.SortOrder, &media.CreatedAt); err != nil {
			rows.Close()
			return Article{}, err
		}
		media.IsCover = cover == 1
		a.Media = append(a.Media, media)
	}
	rows.Close()
	return a, nil
}
func (r *Repository) List(ctx context.Context, slug, title string) ([]Article, error) {
	query := "SELECT id FROM articles WHERE 1=1"
	args := []any{}
	if slug != "" {
		query += " AND slug=?"
		args = append(args, slug)
	}
	if title != "" {
		query += " AND title LIKE ? COLLATE NOCASE"
		args = append(args, "%"+title+"%")
	}
	query += " ORDER BY created_at DESC,id DESC LIMIT 100"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	out := make([]Article, 0, len(ids))
	for _, id := range ids {
		a, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (r *Repository) ListPublished(ctx context.Context, categorySlug string) ([]Article, error) {
	query := "SELECT DISTINCT a.id FROM articles a"
	args := []any{}
	if categorySlug != "" {
		query += " JOIN article_categories ac ON ac.article_id=a.id JOIN categories c ON c.id=ac.category_id"
	}
	query += " WHERE a.status='published'"
	if categorySlug != "" {
		query += " AND c.slug=?"
		args = append(args, categorySlug)
	}
	query += " ORDER BY a.created_at DESC,a.id DESC LIMIT 100"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	out := make([]Article, 0, len(ids))
	for _, id := range ids {
		article, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, article)
	}
	return out, nil
}

func (r *Repository) GetPublishedBySlug(ctx context.Context, slug string) (Article, error) {
	article, err := r.GetBySlug(ctx, slug)
	if err != nil {
		return Article{}, err
	}
	if article.Status != "published" {
		return Article{}, ErrNotFound
	}
	return article, nil
}

func (r *Repository) GetCategoryBySlug(ctx context.Context, slug string) (Category, error) {
	var c Category
	err := r.db.QueryRowContext(ctx, "SELECT id,slug,name,description,parent_id,created_at FROM categories WHERE slug=?", slug).Scan(&c.ID, &c.Slug, &c.Name, &c.Description, &c.ParentID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Category{}, ErrNotFound
	}
	return c, err
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM articles WHERE id=?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteAndQueueMedia(ctx context.Context, id int64) ([]ObjectDeletion, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM articles WHERE id=?", id).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 1 {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO object_deletions(article_id,object_key,created_at) SELECT article_id,object_key,? FROM media WHERE article_id=?`, now, id); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM articles WHERE id=?", id); err != nil {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,article_id,object_key FROM object_deletions WHERE article_id=? ORDER BY id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ObjectDeletion{}
	for rows.Next() {
		var item ObjectDeletion
		if err = rows.Scan(&item.ID, &item.ArticleID, &item.ObjectKey); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 && exists == 0 {
		return nil, ErrNotFound
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) CompleteObjectDeletion(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM object_deletions WHERE id=?", id)
	return err
}

func (r *Repository) CreateCategory(ctx context.Context, in CategoryInput) (Category, error) {
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Name = strings.TrimSpace(in.Name)
	if err := ValidateCategory(in); err != nil {
		return Category{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.ExecContext(ctx, "INSERT INTO categories(slug,name,description,parent_id,created_at) VALUES(?,?,?,?,?)", in.Slug, in.Name, in.Description, in.ParentID, now)
	if unique(err) {
		return Category{}, ErrConflict
	}
	if err != nil {
		return Category{}, err
	}
	id, _ := result.LastInsertId()
	var c Category
	err = r.db.QueryRowContext(ctx, "SELECT id,slug,name,description,parent_id,created_at FROM categories WHERE id=?", id).Scan(&c.ID, &c.Slug, &c.Name, &c.Description, &c.ParentID, &c.CreatedAt)
	return c, err
}
func (r *Repository) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,slug,name,description,parent_id,created_at FROM categories ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Slug, &c.Name, &c.Description, &c.ParentID, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *Repository) CreateMedia(ctx context.Context, in MediaInput) (Media, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Media{}, err
	}
	defer tx.Rollback()
	var slug string
	if err = tx.QueryRowContext(ctx, "SELECT slug FROM articles WHERE id=?", in.ArticleID).Scan(&slug); errors.Is(err, sql.ErrNoRows) {
		return Media{}, ErrNotFound
	} else if err != nil {
		return Media{}, err
	}
	if in.IsCover {
		if _, err = tx.ExecContext(ctx, "UPDATE media SET is_cover=0 WHERE article_id=?", in.ArticleID); err != nil {
			return Media{}, err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `INSERT INTO media(article_id,type,url,object_key,source_url,caption,alt_text,license,is_cover,sort_order,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, in.ArticleID, in.Type, in.URL, in.ObjectKey, in.SourceURL, in.Caption, in.AltText, in.License, in.IsCover, in.SortOrder, now)
	if err != nil {
		return Media{}, err
	}
	id, _ := result.LastInsertId()
	if err = tx.Commit(); err != nil {
		return Media{}, err
	}
	return r.GetMedia(ctx, id)
}
func (r *Repository) GetMedia(ctx context.Context, id int64) (Media, error) {
	var m Media
	var cover int
	err := r.db.QueryRowContext(ctx, `SELECT id,article_id,type,url,object_key,source_url,caption,alt_text,license,is_cover,sort_order,created_at FROM media WHERE id=?`, id).Scan(&m.ID, &m.ArticleID, &m.Type, &m.URL, &m.ObjectKey, &m.SourceURL, &m.Caption, &m.AltText, &m.License, &cover, &m.SortOrder, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Media{}, ErrNotFound
	}
	m.IsCover = cover == 1
	return m, err
}
func (r *Repository) DeleteMedia(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM media WHERE id=?", id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ListTopCategories(ctx context.Context) ([]Category, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,slug,name,description,parent_id,created_at FROM categories WHERE parent_id IS NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var category Category
		if err := rows.Scan(&category.ID, &category.Slug, &category.Name, &category.Description, &category.ParentID, &category.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, category)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func unique(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
