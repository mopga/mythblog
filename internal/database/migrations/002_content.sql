CREATE TABLE articles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  slug TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  subtitle TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  body_markdown TEXT NOT NULL,
  type TEXT NOT NULL DEFAULT 'other',
  status TEXT NOT NULL DEFAULT 'published',
  credibility TEXT NOT NULL DEFAULT 'unknown',
  event_date_text TEXT NOT NULL DEFAULT '',
  location_text TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX idx_articles_created ON articles(created_at DESC);
CREATE INDEX idx_articles_title ON articles(title);

CREATE TABLE categories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  parent_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE article_categories (
  article_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
  category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
  PRIMARY KEY(article_id, category_id)
);

CREATE TABLE sources (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  url TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL DEFAULT '',
  publisher TEXT NOT NULL DEFAULT '',
  published_at TEXT,
  source_type TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE TABLE article_sources (
  article_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
  source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  PRIMARY KEY(article_id, source_id)
);

CREATE TABLE article_relations (
  article_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
  related_article_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
  relation TEXT NOT NULL,
  PRIMARY KEY(article_id, related_article_id, relation),
  CHECK(article_id <> related_article_id)
);
