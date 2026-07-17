package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"obdurate/internal/model"
)

// ResolveDeveloper finds a developer by id, email, username, or slack_id.
func (s *Store) ResolveDeveloper(ref string) (*model.Developer, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("%w: empty developer reference", ErrInvalidInput)
	}

	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		d, err := s.GetDeveloper(id)
		if err == nil {
			return d, nil
		}
		if err != ErrNotFound {
			return nil, err
		}
	}

	const q = `
SELECT id, name, email, username, slack_id, role, created_at, updated_at
FROM developers
WHERE lower(email) = lower(?)
   OR lower(username) = lower(?)
   OR (slack_id IS NOT NULL AND lower(slack_id) = lower(?))
LIMIT 1`
	return s.scanDeveloper(s.db.QueryRow(q, ref, ref, ref))
}

func (s *Store) GetDeveloper(id int64) (*model.Developer, error) {
	const q = `
SELECT id, name, email, username, slack_id, role, created_at, updated_at
FROM developers WHERE id = ?`
	return s.scanDeveloper(s.db.QueryRow(q, id))
}

func (s *Store) scanDeveloper(row *sql.Row) (*model.Developer, error) {
	var d model.Developer
	var slack sql.NullString
	var created, updated string
	err := row.Scan(&d.ID, &d.Name, &d.Email, &d.Username, &slack, &d.Role, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.SlackID = strPtr(slack)
	d.CreatedAt = parseTime(created)
	d.UpdatedAt = parseTime(updated)
	return &d, nil
}

func (s *Store) CreateDeveloper(name, email, username string, slackID *string, role model.Role) (*model.Developer, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(email) == "" || strings.TrimSpace(username) == "" {
		return nil, fmt.Errorf("%w: name, email, and username are required", ErrInvalidInput)
	}
	if !model.ValidRole(string(role)) {
		return nil, fmt.Errorf("%w: invalid role %q", ErrInvalidInput, role)
	}
	ts := now()
	const q = `
INSERT INTO developers (name, email, username, slack_id, role, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`
	res, err := s.db.Exec(q, name, email, username, nullString(slackID), string(role), ts, ts)
	if err != nil {
		return nil, wrapUnique(err, "developer")
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetDeveloper(id)
}

type DeveloperUpdate struct {
	Name     *string
	Email    *string
	Username *string
	SlackID  *string // empty string clears
	Role     *model.Role
}

func (s *Store) UpdateDeveloper(ref string, u DeveloperUpdate) (*model.Developer, error) {
	d, err := s.ResolveDeveloper(ref)
	if err != nil {
		return nil, err
	}
	if u.Name != nil {
		d.Name = *u.Name
	}
	if u.Email != nil {
		d.Email = *u.Email
	}
	if u.Username != nil {
		d.Username = *u.Username
	}
	if u.SlackID != nil {
		if *u.SlackID == "" {
			d.SlackID = nil
		} else {
			d.SlackID = u.SlackID
		}
	}
	if u.Role != nil {
		if !model.ValidRole(string(*u.Role)) {
			return nil, fmt.Errorf("%w: invalid role %q", ErrInvalidInput, *u.Role)
		}
		d.Role = *u.Role
	}
	ts := now()
	const q = `
UPDATE developers SET name=?, email=?, username=?, slack_id=?, role=?, updated_at=?
WHERE id=?`
	_, err = s.db.Exec(q, d.Name, d.Email, d.Username, nullString(d.SlackID), string(d.Role), ts, d.ID)
	if err != nil {
		return nil, wrapUnique(err, "developer")
	}
	return s.GetDeveloper(d.ID)
}

func (s *Store) DeleteDeveloper(ref string) error {
	d, err := s.ResolveDeveloper(ref)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM developers WHERE id = ?`, d.ID)
	return err
}

func (s *Store) ListDevelopers() ([]model.Developer, error) {
	const q = `
SELECT id, name, email, username, slack_id, role, created_at, updated_at
FROM developers ORDER BY username`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Developer
	for rows.Next() {
		var d model.Developer
		var slack sql.NullString
		var created, updated string
		if err := rows.Scan(&d.ID, &d.Name, &d.Email, &d.Username, &slack, &d.Role, &created, &updated); err != nil {
			return nil, err
		}
		d.SlackID = strPtr(slack)
		d.CreatedAt = parseTime(created)
		d.UpdatedAt = parseTime(updated)
		out = append(out, d)
	}
	return out, rows.Err()
}

func DeveloperRef(d *model.Developer) string {
	if d == nil {
		return ""
	}
	return d.Username
}
