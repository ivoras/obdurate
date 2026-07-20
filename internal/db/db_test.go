package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestOpenMigratesLegacyActivityTable verifies that a database created before
// the activity.data column existed gains it on Open, with old rows intact.
func TestOpenMigratesLegacyActivityTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	_, err = legacy.Exec(`
CREATE TABLE activity (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id    INTEGER,
    project_id INTEGER,
    board_id   INTEGER,
    actor_id   INTEGER,
    kind       TEXT NOT NULL,
    message    TEXT NOT NULL,
    created_at TEXT NOT NULL
);
INSERT INTO activity (kind, message, created_at) VALUES ('created', 'old row', '2026-01-01T00:00:00Z');`)
	if err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}

	dbh, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer dbh.Close()

	var data sql.NullString
	var msg string
	err = dbh.QueryRow(`SELECT message, data FROM activity WHERE id = 1`).Scan(&msg, &data)
	if err != nil {
		t.Fatalf("select with data column: %v", err)
	}
	if msg != "old row" {
		t.Errorf("message = %q, want old row preserved", msg)
	}
	if data.Valid {
		t.Errorf("legacy row data = %q, want NULL", data.String)
	}

	// Re-opening must not fail or duplicate the column.
	if err := dbh.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	dbh2, err := Open(path)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	defer dbh2.Close()
	var n int
	if err := dbh2.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('activity') WHERE name = 'data'`).Scan(&n); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if n != 1 {
		t.Errorf("data columns = %d, want exactly 1", n)
	}
}

// TestOpenBackfillsTasksFTS verifies that a database with tasks predating
// the tasks_fts table gets those tasks indexed on Open, and that reopening
// doesn't error or duplicate the index.
func TestOpenBackfillsTasksFTS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-tasks.db")
	legacy, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	stmts := []string{
		`CREATE TABLE boards (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE columns (id INTEGER PRIMARY KEY, board_id INTEGER, name TEXT)`,
		`CREATE TABLE tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT, board_id INTEGER NOT NULL,
			column_id INTEGER NOT NULL, title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
			assignee_id INTEGER, priority TEXT NOT NULL DEFAULT 'medium', position INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`INSERT INTO boards (id, name) VALUES (1, 'b')`,
		`INSERT INTO columns (id, board_id, name) VALUES (1, 1, 'Todo')`,
		`INSERT INTO tasks (board_id, column_id, title, description, created_at, updated_at)
		 VALUES (1, 1, 'Preexisting login bug', 'crashes on login', '2020-01-01T00:00:00Z', '2020-01-01T00:00:00Z')`,
	}
	for _, s := range stmts {
		if _, err := legacy.Exec(s); err != nil {
			t.Fatalf("seed legacy schema %q: %v", s, err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}

	dbh, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var count int
	if err := dbh.QueryRow(`SELECT count(*) FROM tasks_fts WHERE tasks_fts MATCH 'login'`).Scan(&count); err != nil {
		t.Fatalf("query tasks_fts: %v", err)
	}
	if count != 1 {
		t.Errorf("backfilled tasks_fts MATCH 'login' count = %d, want 1", count)
	}
	if err := dbh.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopening must not error, and must not duplicate the index entry.
	dbh2, err := Open(path)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	defer dbh2.Close()
	if err := dbh2.QueryRow(`SELECT count(*) FROM tasks_fts WHERE tasks_fts MATCH 'login'`).Scan(&count); err != nil {
		t.Fatalf("query tasks_fts after reopen: %v", err)
	}
	if count != 1 {
		t.Errorf("after reopen, match count = %d, want 1", count)
	}
	var docCount int
	if err := dbh2.QueryRow(`SELECT count(*) FROM tasks_fts_docsize`).Scan(&docCount); err != nil {
		t.Fatalf("docsize count: %v", err)
	}
	if docCount != 1 {
		t.Errorf("tasks_fts_docsize count = %d, want 1 (no duplicate reindex on reopen)", docCount)
	}
}

func TestOpenCreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dirs", "obd.db")
	dbh, err := Open(path)
	if err != nil {
		t.Fatalf("Open with missing parents: %v", err)
	}
	defer dbh.Close()
	if err := dbh.Ping(); err != nil {
		t.Errorf("ping: %v", err)
	}
}
