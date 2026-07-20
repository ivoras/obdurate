package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	if err := ensureColumn(db, "activity", "data", "TEXT"); err != nil {
		return fmt.Errorf("migrate activity.data: %w", err)
	}
	if err := backfillTasksFTS(db); err != nil {
		return fmt.Errorf("backfill tasks_fts: %w", err)
	}
	return nil
}

// backfillTasksFTS populates tasks_fts for databases that already had rows
// in tasks before the FTS5 table (and its sync triggers) existed; triggers
// only cover writes made after they were created. Uses FTS5's 'rebuild'
// command rather than a manual per-row INSERT: manual INSERTs into an
// external-content table were observed to be silently unreliable here (the
// row "counts" but MATCH never finds it), whereas 'rebuild' is SQLite's own
// documented, robust way to resync the index from the content table.
//
// The populated check queries tasks_fts_docsize (a real shadow table, one
// row per indexed doc) rather than "SELECT count(*) FROM tasks_fts": for an
// external-content table, a plain (non-MATCH) count on the FTS table itself
// passes through to the content table's row count regardless of whether
// the index was ever built, so it can't be used as a populated signal.
func backfillTasksFTS(db *sql.DB) error {
	var docCount int
	if err := db.QueryRow(`SELECT count(*) FROM tasks_fts_docsize`).Scan(&docCount); err != nil {
		return err
	}
	if docCount > 0 {
		return nil
	}
	var taskCount int
	if err := db.QueryRow(`SELECT count(*) FROM tasks`).Scan(&taskCount); err != nil {
		return err
	}
	if taskCount == 0 {
		return nil
	}
	_, err := db.Exec(`INSERT INTO tasks_fts(tasks_fts) VALUES('rebuild')`)
	return err
}

// ensureColumn adds a column to a pre-existing table; CREATE TABLE IF NOT
// EXISTS in schema.sql does not alter tables created by older versions.
func ensureColumn(db *sql.DB, table, column, ddl string) error {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	// Close before ALTER: the pool has a single connection.
	rows.Close()
	if found {
		return nil
	}
	_, err = db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, ddl))
	return err
}
