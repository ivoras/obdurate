package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"obdurate/internal/model"
)

var defaultColumns = []string{"Todo", "Doing", "Done"}

func (s *Store) CreateBoard(projectRef, name, description string) (*model.Board, error) {
	p, err := s.ResolveProject(projectRef)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: board name is required", ErrInvalidInput)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	ts := now()
	res, err := tx.Exec(
		`INSERT INTO boards (project_id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		p.ID, name, description, ts, ts,
	)
	if err != nil {
		return nil, wrapUnique(err, "board")
	}
	boardID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	for i, col := range defaultColumns {
		_, err := tx.Exec(
			`INSERT INTO columns (board_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			boardID, col, i, ts, ts,
		)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetBoard(boardID)
}

func (s *Store) GetBoard(id int64) (*model.Board, error) {
	const q = `SELECT id, project_id, name, description, created_at, updated_at FROM boards WHERE id = ?`
	return s.scanBoard(s.db.QueryRow(q, id))
}

// ResolveBoard accepts "id", "project/board", or unique board name.
func (s *Store) ResolveBoard(ref string) (*model.Board, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("%w: empty board reference", ErrInvalidInput)
	}
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		b, err := s.GetBoard(id)
		if err == nil {
			return b, nil
		}
		if err != ErrNotFound {
			return nil, err
		}
	}
	if parts := strings.SplitN(ref, "/", 2); len(parts) == 2 {
		p, err := s.ResolveProject(parts[0])
		if err != nil {
			return nil, err
		}
		const q = `
SELECT id, project_id, name, description, created_at, updated_at
FROM boards WHERE project_id = ? AND lower(name) = lower(?)`
		return s.scanBoard(s.db.QueryRow(q, p.ID, parts[1]))
	}

	const q = `SELECT id, project_id, name, description, created_at, updated_at FROM boards WHERE lower(name) = lower(?)`
	rows, err := s.db.Query(q, ref)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var matches []model.Board
	for rows.Next() {
		var b model.Board
		var created, updated string
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.Name, &b.Description, &created, &updated); err != nil {
			return nil, err
		}
		b.CreatedAt = parseTime(created)
		b.UpdatedAt = parseTime(updated)
		matches = append(matches, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, ErrNotFound
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("%w: board name %q is ambiguous; use project/board", ErrConflict, ref)
	}
	return &matches[0], nil
}

func (s *Store) scanBoard(row *sql.Row) (*model.Board, error) {
	var b model.Board
	var created, updated string
	err := row.Scan(&b.ID, &b.ProjectID, &b.Name, &b.Description, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	b.CreatedAt = parseTime(created)
	b.UpdatedAt = parseTime(updated)
	return &b, nil
}

type BoardUpdate struct {
	Name        *string
	Description *string
}

func (s *Store) UpdateBoard(ref string, u BoardUpdate) (*model.Board, error) {
	b, err := s.ResolveBoard(ref)
	if err != nil {
		return nil, err
	}
	if u.Name != nil {
		b.Name = strings.TrimSpace(*u.Name)
		if b.Name == "" {
			return nil, fmt.Errorf("%w: board name cannot be empty", ErrInvalidInput)
		}
	}
	if u.Description != nil {
		b.Description = *u.Description
	}
	ts := now()
	_, err = s.db.Exec(`UPDATE boards SET name=?, description=?, updated_at=? WHERE id=?`,
		b.Name, b.Description, ts, b.ID)
	if err != nil {
		return nil, wrapUnique(err, "board")
	}
	return s.GetBoard(b.ID)
}

func (s *Store) DeleteBoard(ref string) error {
	b, err := s.ResolveBoard(ref)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM boards WHERE id = ?`, b.ID)
	return err
}

func (s *Store) ListBoards(projectRef string) ([]model.Board, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if strings.TrimSpace(projectRef) == "" {
		rows, err = s.db.Query(`SELECT id, project_id, name, description, created_at, updated_at FROM boards ORDER BY project_id, name`)
	} else {
		p, err2 := s.ResolveProject(projectRef)
		if err2 != nil {
			return nil, err2
		}
		rows, err = s.db.Query(
			`SELECT id, project_id, name, description, created_at, updated_at FROM boards WHERE project_id = ? ORDER BY name`,
			p.ID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Board{}
	for rows.Next() {
		var b model.Board
		var created, updated string
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.Name, &b.Description, &created, &updated); err != nil {
			return nil, err
		}
		b.CreatedAt = parseTime(created)
		b.UpdatedAt = parseTime(updated)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) ListColumns(boardID int64) ([]model.Column, error) {
	const q = `
SELECT id, board_id, name, position, created_at, updated_at
FROM columns WHERE board_id = ? ORDER BY position`
	rows, err := s.db.Query(q, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Column{}
	for rows.Next() {
		var c model.Column
		var created, updated string
		if err := rows.Scan(&c.ID, &c.BoardID, &c.Name, &c.Position, &created, &updated); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTime(created)
		c.UpdatedAt = parseTime(updated)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ResolveColumn(boardID int64, ref string) (*model.Column, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("%w: empty column reference", ErrInvalidInput)
	}
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		const q = `
SELECT id, board_id, name, position, created_at, updated_at
FROM columns WHERE id = ? AND board_id = ?`
		c, err := s.scanColumn(s.db.QueryRow(q, id, boardID))
		if err == nil {
			return c, nil
		}
		if err != ErrNotFound {
			return nil, err
		}
	}
	const q = `
SELECT id, board_id, name, position, created_at, updated_at
FROM columns WHERE board_id = ? AND lower(name) = lower(?)`
	return s.scanColumn(s.db.QueryRow(q, boardID, ref))
}

func (s *Store) scanColumn(row *sql.Row) (*model.Column, error) {
	var c model.Column
	var created, updated string
	err := row.Scan(&c.ID, &c.BoardID, &c.Name, &c.Position, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt = parseTime(created)
	c.UpdatedAt = parseTime(updated)
	return &c, nil
}

func (s *Store) AddColumn(boardRef, name string, position *int) (*model.Column, error) {
	b, err := s.ResolveBoard(boardRef)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: column name is required", ErrInvalidInput)
	}

	var max sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(position) FROM columns WHERE board_id = ?`, b.ID).Scan(&max); err != nil {
		return nil, err
	}
	end := 0
	if max.Valid {
		end = int(max.Int64) + 1
	}
	pos := end
	if position != nil {
		// Clamp to [0, end] so explicit positions cannot go negative or leave gaps.
		pos = *position
		if pos < 0 {
			pos = 0
		}
		if pos > end {
			pos = end
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Shift columns at or after position.
	_, err = tx.Exec(`UPDATE columns SET position = position + 1 WHERE board_id = ? AND position >= ?`, b.ID, pos)
	if err != nil {
		return nil, err
	}

	ts := now()
	res, err := tx.Exec(
		`INSERT INTO columns (board_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		b.ID, name, pos, ts, ts,
	)
	if err != nil {
		return nil, wrapUnique(err, "column")
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.scanColumn(s.db.QueryRow(
		`SELECT id, board_id, name, position, created_at, updated_at FROM columns WHERE id = ?`, id,
	))
}

func (s *Store) RenameColumn(boardRef, columnRef, newName string) (*model.Column, error) {
	b, err := s.ResolveBoard(boardRef)
	if err != nil {
		return nil, err
	}
	c, err := s.ResolveColumn(b.ID, columnRef)
	if err != nil {
		return nil, err
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return nil, fmt.Errorf("%w: column name is required", ErrInvalidInput)
	}
	ts := now()
	_, err = s.db.Exec(`UPDATE columns SET name=?, updated_at=? WHERE id=?`, newName, ts, c.ID)
	if err != nil {
		return nil, wrapUnique(err, "column")
	}
	return s.ResolveColumn(b.ID, strconv.FormatInt(c.ID, 10))
}

func (s *Store) ReorderColumn(boardRef, columnRef string, newPos int) (*model.Column, error) {
	b, err := s.ResolveBoard(boardRef)
	if err != nil {
		return nil, err
	}
	c, err := s.ResolveColumn(b.ID, columnRef)
	if err != nil {
		return nil, err
	}
	cols, err := s.ListColumns(b.ID)
	if err != nil {
		return nil, err
	}
	if newPos < 0 {
		newPos = 0
	}
	if newPos >= len(cols) {
		newPos = len(cols) - 1
	}
	if c.Position == newPos {
		return c, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Temporary positions to avoid UNIQUE conflicts.
	for i, col := range cols {
		if _, err := tx.Exec(`UPDATE columns SET position = ? WHERE id = ?`, 10000+i, col.ID); err != nil {
			return nil, err
		}
	}

	// Reorder in memory.
	var ordered []model.Column
	for _, col := range cols {
		if col.ID != c.ID {
			ordered = append(ordered, col)
		}
	}
	if newPos > len(ordered) {
		newPos = len(ordered)
	}
	ordered = append(ordered[:newPos], append([]model.Column{*c}, ordered[newPos:]...)...)

	ts := now()
	for i, col := range ordered {
		if _, err := tx.Exec(`UPDATE columns SET position = ?, updated_at = ? WHERE id = ?`, i, ts, col.ID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ResolveColumn(b.ID, strconv.FormatInt(c.ID, 10))
}

func (s *Store) DeleteColumn(boardRef, columnRef string) error {
	b, err := s.ResolveBoard(boardRef)
	if err != nil {
		return err
	}
	c, err := s.ResolveColumn(b.ID, columnRef)
	if err != nil {
		return err
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE column_id = ?`, c.ID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: column has %d task(s); move or delete them first", ErrConflict, count)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM columns WHERE id = ?`, c.ID); err != nil {
		return err
	}

	rows, err := tx.Query(`SELECT id FROM columns WHERE board_id = ? ORDER BY position`, b.ID)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()

	// Avoid UNIQUE(board_id, position) collisions while compacting.
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE columns SET position = ? WHERE id = ?`, 10000+i, id); err != nil {
			return err
		}
	}
	ts := now()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE columns SET position = ?, updated_at = ? WHERE id = ?`, i, ts, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
