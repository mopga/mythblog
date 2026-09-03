CREATE TABLE sources_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  url TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  publisher TEXT NOT NULL DEFAULT '',
  published_at TEXT,
  source_type TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE article_sources_new (
  article_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
  source_id INTEGER NOT NULL REFERENCES sources_new(id) ON DELETE CASCADE,
  PRIMARY KEY(article_id, source_id)
);

INSERT INTO sources_new(id, url, title, publisher, published_at, source_type, created_at)
SELECT ROW_NUMBER() OVER (ORDER BY x.article_id, x.source_id),
       s.url, s.title, s.publisher, s.published_at, s.source_type, s.created_at
FROM article_sources x
JOIN sources s ON s.id = x.source_id;

INSERT INTO article_sources_new(article_id, source_id)
SELECT article_id, ROW_NUMBER() OVER (ORDER BY article_id, source_id)
FROM article_sources;

DROP TABLE article_sources;
DROP TABLE sources;
ALTER TABLE sources_new RENAME TO sources;
ALTER TABLE article_sources_new RENAME TO article_sources;

CREATE TRIGGER cleanup_orphan_source
AFTER DELETE ON article_sources
BEGIN
  DELETE FROM sources
  WHERE id = OLD.source_id
    AND NOT EXISTS (SELECT 1 FROM article_sources WHERE source_id = OLD.source_id);
END;
