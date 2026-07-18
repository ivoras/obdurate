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

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	ts := now()
	const q = `
INSERT INTO developers (name, email, username, slack_id, role, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`
	res, err := tx.Exec(q, name, email, username, nullString(slackID), string(role), ts, ts)
	if err != nil {
		return nil, wrapUnique(err, "developer")
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	var slack any
	if slackID != nil && *slackID != "" {
		slack = *slackID
	}
	data := map[string]any{"entity": "developer", "developer": map[string]any{
		"id": id, "name": name, "email": email, "username": username,
		"slack_id": slack, "role": string(role),
	}}
	msg := fmt.Sprintf("created developer %q", username)
	if err := s.addActivityTx(tx, nil, nil, nil, nil, model.ActivityCreated, msg, data); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
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
	var msgs []string
	changed := map[string]any{}
	if u.Name != nil {
		name := strings.TrimSpace(*u.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name cannot be empty", ErrInvalidInput)
		}
		if name != d.Name {
			msgs = append(msgs, fmt.Sprintf("name %q → %q", d.Name, name))
			changed["name"] = fieldChange(d.Name, name)
			d.Name = name
		}
	}
	if u.Email != nil {
		email := strings.TrimSpace(*u.Email)
		if email == "" {
			return nil, fmt.Errorf("%w: email cannot be empty", ErrInvalidInput)
		}
		if email != d.Email {
			msgs = append(msgs, fmt.Sprintf("email %q → %q", d.Email, email))
			changed["email"] = fieldChange(d.Email, email)
			d.Email = email
		}
	}
	if u.Username != nil {
		username := strings.TrimSpace(*u.Username)
		if username == "" {
			return nil, fmt.Errorf("%w: username cannot be empty", ErrInvalidInput)
		}
		if username != d.Username {
			msgs = append(msgs, fmt.Sprintf("username %q → %q", d.Username, username))
			changed["username"] = fieldChange(d.Username, username)
			d.Username = username
		}
	}
	if u.SlackID != nil {
		var oldSlack any
		if d.SlackID != nil {
			oldSlack = *d.SlackID
		}
		if *u.SlackID == "" {
			if d.SlackID != nil {
				msgs = append(msgs, "slack id cleared")
				changed["slack_id"] = fieldChange(oldSlack, nil)
				d.SlackID = nil
			}
		} else if d.SlackID == nil || *d.SlackID != *u.SlackID {
			msgs = append(msgs, fmt.Sprintf("slack id → %q", *u.SlackID))
			changed["slack_id"] = fieldChange(oldSlack, *u.SlackID)
			d.SlackID = u.SlackID
		}
	}
	if u.Role != nil {
		if !model.ValidRole(string(*u.Role)) {
			return nil, fmt.Errorf("%w: invalid role %q", ErrInvalidInput, *u.Role)
		}
		if *u.Role != d.Role {
			msgs = append(msgs, fmt.Sprintf("role %s → %s", d.Role, *u.Role))
			changed["role"] = fieldChange(string(d.Role), string(*u.Role))
			d.Role = *u.Role
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	ts := now()
	const q = `
UPDATE developers SET name=?, email=?, username=?, slack_id=?, role=?, updated_at=?
WHERE id=?`
	_, err = tx.Exec(q, d.Name, d.Email, d.Username, nullString(d.SlackID), string(d.Role), ts, d.ID)
	if err != nil {
		return nil, wrapUnique(err, "developer")
	}
	if len(changed) > 0 {
		data := map[string]any{"entity": "developer", "developer_id": d.ID, "changes": changed}
		msg := fmt.Sprintf("updated developer %q: %s", d.Username, strings.Join(msgs, "; "))
		if err := s.addActivityTx(tx, nil, nil, nil, nil, model.ActivityUpdated, msg, data); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetDeveloper(d.ID)
}

func (s *Store) DeleteDeveloper(ref string) error {
	d, err := s.ResolveDeveloper(ref)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// actor_id is ON DELETE SET NULL; preserve authorship in the payload so
	// history keeps saying who did what after the developer is gone.
	if _, err := tx.Exec(
		`UPDATE activity SET data = json_set(COALESCE(data, '{}'), '$.actor', ?) WHERE actor_id = ?`,
		d.Username, d.ID,
	); err != nil {
		return err
	}

	data := map[string]any{"entity": "developer", "developer": developerSnapshot(d)}
	msg := fmt.Sprintf("deleted developer %q (#%d)", d.Username, d.ID)
	if err := s.addActivityTx(tx, nil, nil, nil, nil, model.ActivityDeleted, msg, data); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM developers WHERE id = ?`, d.ID); err != nil {
		return err
	}
	return tx.Commit()
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
	out := []model.Developer{}
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
