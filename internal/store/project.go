package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"obdurate/internal/model"
)

// EnsureDefaults seeds a "Default" project with a "main" board (and the
// standard Todo/Doing/Done columns) when the database contains no projects
// at all. Called on startup so unspecified tasks always have a home; a
// deliberately deleted Default project is not recreated once other projects
// exist.
func (s *Store) EnsureDefaults() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := s.CreateProject("default", "Default project", ""); err != nil && !errors.Is(err, ErrAlreadyExists) {
		return err
	}
	if _, err := s.CreateBoard("default", "main", "Default board", ""); err != nil && !errors.Is(err, ErrAlreadyExists) {
		return err
	}
	return nil
}

func (s *Store) CreateProject(name, description, actorRef string) (*model.Project, error) {
	name, err := normalizeSlug(name, "project")
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
	const q = `INSERT INTO projects (name, description, created_at, updated_at) VALUES (?, ?, ?, ?)`
	res, err := tx.Exec(q, name, description, ts, ts)
	if err != nil {
		return nil, wrapUnique(err, "project")
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	data := map[string]any{
		"entity":  "project",
		"project": map[string]any{"id": id, "name": name, "description": description},
	}
	msg := fmt.Sprintf("created project %q", name)
	if err := s.addActivityTx(tx, nil, &id, nil, actorID, model.ActivityCreated, msg, data); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetProject(id)
}

func (s *Store) GetProject(id int64) (*model.Project, error) {
	const q = `SELECT id, name, description, created_at, updated_at FROM projects WHERE id = ?`
	return s.scanProject(s.db.QueryRow(q, id))
}

func (s *Store) ResolveProject(ref string) (*model.Project, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("%w: empty project reference", ErrInvalidInput)
	}
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		p, err := s.GetProject(id)
		if err == nil {
			return p, nil
		}
		if err != ErrNotFound {
			return nil, err
		}
	}
	const q = `SELECT id, name, description, created_at, updated_at FROM projects WHERE lower(name) = lower(?)`
	return s.scanProject(s.db.QueryRow(q, ref))
}

func (s *Store) scanProject(row *sql.Row) (*model.Project, error) {
	var p model.Project
	var created, updated string
	err := row.Scan(&p.ID, &p.Name, &p.Description, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.CreatedAt = parseTime(created)
	p.UpdatedAt = parseTime(updated)
	return &p, nil
}

type ProjectUpdate struct {
	Name        *string
	Description *string
	ActorRef    string
}

func (s *Store) UpdateProject(ref string, u ProjectUpdate) (*model.Project, error) {
	p, err := s.ResolveProject(ref)
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
		name, err := normalizeSlug(*u.Name, "project")
		if err != nil {
			return nil, err
		}
		if name != p.Name {
			msgs = append(msgs, fmt.Sprintf("name %q → %q", p.Name, name))
			changed["name"] = fieldChange(p.Name, name)
			p.Name = name
		}
	}
	if u.Description != nil && *u.Description != p.Description {
		msgs = append(msgs, "description updated")
		changed["description"] = fieldChange(p.Description, *u.Description)
		p.Description = *u.Description
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	ts := now()
	_, err = tx.Exec(`UPDATE projects SET name=?, description=?, updated_at=? WHERE id=?`,
		p.Name, p.Description, ts, p.ID)
	if err != nil {
		return nil, wrapUnique(err, "project")
	}
	if len(changed) > 0 {
		data := map[string]any{"entity": "project", "project_id": p.ID, "changes": changed}
		msg := "updated project: " + strings.Join(msgs, "; ")
		if err := s.addActivityTx(tx, nil, &p.ID, nil, actorID, model.ActivityUpdated, msg, data); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetProject(p.ID)
}

func (s *Store) DeleteProject(ref, actorRef string) error {
	p, err := s.ResolveProject(ref)
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

	// Preserve the project's entire activity history, which the project_id
	// cascade would otherwise delete: detach each foreign key into the JSON
	// payload before removing the project.
	detach := []struct{ column, cond string }{
		{"task_id", "project_id = ? AND task_id IS NOT NULL"},
		{"board_id", "project_id = ? AND board_id IS NOT NULL"},
		{"project_id", "project_id = ?"},
	}
	for _, d := range detach {
		q := fmt.Sprintf(
			`UPDATE activity SET data = json_set(COALESCE(data, '{}'), '$.%s', %s), %s = NULL WHERE %s`,
			d.column, d.column, d.column, d.cond,
		)
		if _, err := tx.Exec(q, p.ID); err != nil {
			return err
		}
	}

	data := map[string]any{"entity": "project", "project": projectSnapshot(p)}
	msg := fmt.Sprintf("deleted project %q (#%d)", p.Name, p.ID)
	if err := s.addActivityTx(tx, nil, nil, nil, actorID, model.ActivityDeleted, msg, data); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM projects WHERE id = ?`, p.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListProjects() ([]model.Project, error) {
	const q = `SELECT id, name, description, created_at, updated_at FROM projects ORDER BY name`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Project{}
	for rows.Next() {
		var p model.Project
		var created, updated string
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &created, &updated); err != nil {
			return nil, err
		}
		p.CreatedAt = parseTime(created)
		p.UpdatedAt = parseTime(updated)
		out = append(out, p)
	}
	return out, rows.Err()
}
