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
