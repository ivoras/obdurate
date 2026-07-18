package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"obdurate/internal/model"
)

var defaultColumns = []string{"Todo", "Doing", "Done"}

func (s *Store) CreateBoard(projectRef, name, description, actorRef string) (*model.Board, error) {
	p, err := s.ResolveProject(projectRef)
	if err != nil {
		return nil, err
	}
	name, err = normalizeSlug(name, "board")
	if err != nil {
		return nil, err
	}
	actorID, err := s.resolveActorID(actorRef)
	if err != nil {
		return nil, err
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

	data := map[string]any{
		"entity": "board",
		"board":  map[string]any{"id": boardID, "name": name, "description": description, "project": p.Name},
	}
	msg := fmt.Sprintf("created board %q in project %q", name, p.Name)
	if err := s.addActivityTx(tx, nil, &p.ID, &boardID, actorID, model.ActivityCreated, msg, data); err != nil {
		return nil, err
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
	ActorRef    string
}

func (s *Store) UpdateBoard(ref string, u BoardUpdate) (*model.Board, error) {
	b, err := s.ResolveBoard(ref)
	if err != nil {
		return nil, err
	}
	actorID, err := s.resolveActorID(u.ActorRef)
	if err != nil {
		return nil, err
	}

	var msgs []string
	changed := map[string]any{}
	if u.Name != nil {
		name, err := normalizeSlug(*u.Name, "board")
		if err != nil {
			return nil, err
		}
		if name != b.Name {
			msgs = append(msgs, fmt.Sprintf("name %q → %q", b.Name, name))
			changed["name"] = fieldChange(b.Name, name)
			b.Name = name
		}
	}
	if u.Description != nil && *u.Description != b.Description {
		msgs = append(msgs, "description updated")
		changed["description"] = fieldChange(b.Description, *u.Description)
		b.Description = *u.Description
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	ts := now()
	_, err = tx.Exec(`UPDATE boards SET name=?, description=?, updated_at=? WHERE id=?`,
		b.Name, b.Description, ts, b.ID)
	if err != nil {
		return nil, wrapUnique(err, "board")
	}
	if len(changed) > 0 {
		data := map[string]any{"entity": "board", "board_id": b.ID, "changes": changed}
		msg := "updated board: " + strings.Join(msgs, "; ")
		if err := s.addActivityTx(tx, nil, &b.ProjectID, &b.ID, actorID, model.ActivityUpdated, msg, data); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetBoard(b.ID)
}

func (s *Store) DeleteBoard(ref, actorRef string) error {
	b, err := s.ResolveBoard(ref)
	if err != nil {
		return err
	}
	p, err := s.GetProject(b.ProjectID)
	if err != nil {
		return err
	}
	actorID, err := s.resolveActorID(actorRef)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Preserve the board's activity history, which the board_id cascade (and
	// the cascade of its tasks) would otherwise delete: detach the foreign
	// keys into the JSON payload. Rows keep project_id and stay visible in
	// the project stream.
	if _, err := tx.Exec(
		`UPDATE activity SET data = json_set(COALESCE(data, '{}'), '$.task_id', task_id), task_id = NULL
		 WHERE board_id = ? AND task_id IS NOT NULL`, b.ID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE activity SET data = json_set(COALESCE(data, '{}'), '$.board_id', board_id), board_id = NULL
		 WHERE board_id = ?`, b.ID,
	); err != nil {
		return err
	}

	data := map[string]any{"entity": "board", "board": boardSnapshot(b, p.Name)}
	msg := fmt.Sprintf("deleted board %q (#%d)", b.Name, b.ID)
	if err := s.addActivityTx(tx, nil, &b.ProjectID, nil, actorID, model.ActivityDeleted, msg, data); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM boards WHERE id = ?`, b.ID); err != nil {
		return err
	}
	return tx.Commit()
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

func (s *Store) AddColumn(boardRef, name string, position *int, actorRef string) (*model.Column, error) {
	b, err := s.ResolveBoard(boardRef)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: column name is required", ErrInvalidInput)
	}
	actorID, err := s.resolveActorID(actorRef)
	if err != nil {
		return nil, err
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

	// Shift columns at or after position. Two steps via negative temporaries:
	// UNIQUE(board_id, position) forbids a single in-place +1 update.
	_, err = tx.Exec(`UPDATE columns SET position = -(position + 2) WHERE board_id = ? AND position >= ?`, b.ID, pos)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(`UPDATE columns SET position = -position - 1 WHERE board_id = ? AND position < 0`, b.ID)
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

	data := map[string]any{
		"entity": "column",
		"column": map[string]any{"id": id, "name": name, "position": pos},
	}
	msg := fmt.Sprintf("added column %q at position %d", name, pos)
	if err := s.addActivityTx(tx, nil, &b.ProjectID, &b.ID, actorID, model.ActivityCreated, msg, data); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.scanColumn(s.db.QueryRow(
		`SELECT id, board_id, name, position, created_at, updated_at FROM columns WHERE id = ?`, id,
	))
}

func (s *Store) RenameColumn(boardRef, columnRef, newName, actorRef string) (*model.Column, error) {
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
	actorID, err := s.resolveActorID(actorRef)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	ts := now()
	_, err = tx.Exec(`UPDATE columns SET name=?, updated_at=? WHERE id=?`, newName, ts, c.ID)
	if err != nil {
		return nil, wrapUnique(err, "column")
	}
	if newName != c.Name {
		data := map[string]any{
			"entity": "column", "column_id": c.ID,
			"changes": map[string]any{"name": fieldChange(c.Name, newName)},
		}
		msg := fmt.Sprintf("renamed column %q → %q", c.Name, newName)
		if err := s.addActivityTx(tx, nil, &b.ProjectID, &b.ID, actorID, model.ActivityUpdated, msg, data); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ResolveColumn(b.ID, strconv.FormatInt(c.ID, 10))
}

func (s *Store) ReorderColumn(boardRef, columnRef string, newPos int, actorRef string) (*model.Column, error) {
	b, err := s.ResolveBoard(boardRef)
	if err != nil {
		return nil, err
	}
	c, err := s.ResolveColumn(b.ID, columnRef)
	if err != nil {
		return nil, err
	}
	actorID, err := s.resolveActorID(actorRef)
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

	data := map[string]any{
		"entity": "column",
		"column": map[string]any{"id": c.ID, "name": c.Name},
		"from":   map[string]any{"position": c.Position},
		"to":     map[string]any{"position": newPos},
	}
	msg := fmt.Sprintf("moved column %q from position %d to %d", c.Name, c.Position, newPos)
	if err := s.addActivityTx(tx, nil, &b.ProjectID, &b.ID, actorID, model.ActivityMoved, msg, data); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ResolveColumn(b.ID, strconv.FormatInt(c.ID, 10))
}

func (s *Store) DeleteColumn(boardRef, columnRef, actorRef string) error {
	b, err := s.ResolveBoard(boardRef)
	if err != nil {
		return err
	}
	c, err := s.ResolveColumn(b.ID, columnRef)
	if err != nil {
		return err
	}
	actorID, err := s.resolveActorID(actorRef)
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

	data := map[string]any{"entity": "column", "column": columnSnapshot(c)}
	msg := fmt.Sprintf("deleted column %q", c.Name)
	if err := s.addActivityTx(tx, nil, &b.ProjectID, &b.ID, actorID, model.ActivityDeleted, msg, data); err != nil {
		return err
	}
	return tx.Commit()
}
