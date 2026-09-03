package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrationFourPreservesExistingSourceLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, name := range []string{"migrations/001_bootstrap.sql", "migrations/002_content.sql", "migrations/003_media.sql"} {
		body, err := migrations.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for version := 1; version <= 3; version++ {
		if _, err = db.Exec("INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(?, 'test')", version); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO articles(id,slug,title,body_markdown,created_at,updated_at) VALUES
		(1,'a','A','A','test','test'),(2,'b','B','B','test','test')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO sources(id,url,title,created_at) VALUES(1,'https://example.test/shared','Shared','test')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO article_sources(article_id,source_id) VALUES(1,1),(2,1)`); err != nil {
		t.Fatal(err)
	}
	if err = migrate(db); err != nil {
		t.Fatal(err)
	}
	var sources, links, distinctSources int
	if err = db.QueryRow("SELECT COUNT(*) FROM sources").Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*), COUNT(DISTINCT source_id) FROM article_sources").Scan(&links, &distinctSources); err != nil {
		t.Fatal(err)
	}
	if sources != 2 || links != 2 || distinctSources != 2 {
		t.Fatalf("sources=%d links=%d distinct=%d", sources, links, distinctSources)
	}
	if _, err = db.Exec("DELETE FROM articles WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM sources").Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if sources != 1 {
		t.Fatalf("orphan source after article delete: sources=%d", sources)
	}
}
