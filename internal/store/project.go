package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"obdurate/internal/model"
)

func (s *Store) CreateProject(name, description string) (*model.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: project name is required", ErrInvalidInput)
	}
	ts := now()
	const q = `INSERT INTO projects (name, description, created_at, updated_at) VALUES (?, ?, ?, ?)`
	res, err := s.db.Exec(q, name, description, ts, ts)
	if err != nil {
		return nil, wrapUnique(err, "project")
	}
	id, err := res.LastInsertId()
	if err != nil {
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
}

func (s *Store) UpdateProject(ref string, u ProjectUpdate) (*model.Project, error) {
	p, err := s.ResolveProject(ref)
	if err != nil {
		return nil, err
	}
	if u.Name != nil {
		p.Name = strings.TrimSpace(*u.Name)
		if p.Name == "" {
			return nil, fmt.Errorf("%w: project name cannot be empty", ErrInvalidInput)
		}
	}
	if u.Description != nil {
		p.Description = *u.Description
	}
	ts := now()
	_, err = s.db.Exec(`UPDATE projects SET name=?, description=?, updated_at=? WHERE id=?`,
		p.Name, p.Description, ts, p.ID)
	if err != nil {
		return nil, wrapUnique(err, "project")
	}
	return s.GetProject(p.ID)
}

func (s *Store) DeleteProject(ref string) error {
	p, err := s.ResolveProject(ref)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM projects WHERE id = ?`, p.ID)
	return err
}

func (s *Store) ListProjects() ([]model.Project, error) {
	const q = `SELECT id, name, description, created_at, updated_at FROM projects ORDER BY name`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Project
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
