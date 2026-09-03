CREATE TABLE media (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  article_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  url TEXT NOT NULL,
  object_key TEXT NOT NULL UNIQUE,
  source_url TEXT NOT NULL DEFAULT '',
  caption TEXT NOT NULL DEFAULT '',
  alt_text TEXT NOT NULL DEFAULT '',
  license TEXT NOT NULL DEFAULT '',
  is_cover INTEGER NOT NULL DEFAULT 0 CHECK(is_cover IN (0,1)),
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_media_article ON media(article_id, is_cover DESC, sort_order, id);
