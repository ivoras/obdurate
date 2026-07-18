package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"obdurate/internal/model"
)

// Activity data payload shapes. Every payload carries "entity" naming the
// affected object kind: "task", "project", "board", "column", "developer".
//
// entity "task", by kind:
//
//	created:   {"task": <task snapshot>}
//	updated:   {"changes": {"<field>": {"old": ..., "new": ...}, ...}}
//	moved:     {"from": {"column": ..., "column_id": ..., "position": ...},
//	            "to":   {"column": ..., "column_id": ..., "position": ...}}
//	deleted:   {"task": <task snapshot>}         (state just before deletion)
//	watched:   {"developer": <username>}
//	unwatched: {"developer": <username>}
//	commented: no payload (message is the comment text)
//
// A task snapshot is: {"id", "title", "description", "column", "column_id",
// "priority", "position", "assignee" (username or null), "tags", "watchers"}.
//
// Other entities:
//
//	project created/deleted:   {"project": {"id", "name", "description"}}
//	project updated:           {"project_id": ..., "changes": {...}}
//	board created/deleted:     {"board": {"id", "name", "description", "project"}}
//	board updated:             {"board_id": ..., "changes": {...}}
//	column created/deleted:    {"column": {"id", "name", "position"}}
//	column updated:            {"column_id": ..., "changes": {...}}
//	column moved:              {"column": {...}, "from": {"position": ...}, "to": {"position": ...}}
//	developer created/deleted: {"developer": {"id", "name", "email", "username", "slack_id", "role"}}
//	developer updated:         {"developer_id": ..., "changes": {...}}
//
// Deletions detach rather than cascade the deleted object's activity rows:
// the row's task_id/board_id/project_id foreign keys move into the payload
// as data.task_id / data.board_id / data.project_id, and a deleted
// developer's authorship is preserved as data.actor (username).

// taskSnapshot captures the externally meaningful state of a task for
// activity payloads (created/deleted), enough to reconstruct it.
func taskSnapshot(t *model.Task) map[string]any {
	var assignee any
	if t.AssigneeRef != "" {
		assignee = t.AssigneeRef
	}
	tags := t.Tags
	if tags == nil {
		tags = []string{}
	}
	watchers := t.WatcherRefs
	if watchers == nil {
		watchers = []string{}
	}
	return map[string]any{
		"id":          t.ID,
		"title":       t.Title,
		"description": t.Description,
		"column":      t.ColumnName,
		"column_id":   t.ColumnID,
		"priority":    string(t.Priority),
		"position":    t.Position,
		"assignee":    assignee,
		"tags":        tags,
		"watchers":    watchers,
	}
}

// fieldChange is the {"old": ..., "new": ...} element of an "updated" payload.
func fieldChange(old, new any) map[string]any {
	return map[string]any{"old": old, "new": new}
}

func projectSnapshot(p *model.Project) map[string]any {
	return map[string]any{"id": p.ID, "name": p.Name, "description": p.Description}
}

func boardSnapshot(b *model.Board, projectName string) map[string]any {
	return map[string]any{"id": b.ID, "name": b.Name, "description": b.Description, "project": projectName}
}

func columnSnapshot(c *model.Column) map[string]any {
	return map[string]any{"id": c.ID, "name": c.Name, "position": c.Position}
}

func developerSnapshot(d *model.Developer) map[string]any {
	var slack any
	if d.SlackID != nil {
		slack = *d.SlackID
	}
	return map[string]any{
		"id": d.ID, "name": d.Name, "email": d.Email,
		"username": d.Username, "slack_id": slack, "role": string(d.Role),
	}
}

// resolveActorID resolves an optional --by actor reference for activity
// attribution; an empty ref means no actor.
func (s *Store) resolveActorID(ref string) (*int64, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, nil
	}
	d, err := s.ResolveDeveloper(ref)
	if err != nil {
		return nil, fmt.Errorf("actor: %w", err)
	}
	return &d.ID, nil
}

func marshalActivityData(data any) (any, error) {
	if data == nil {
		return nil, nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal activity data: %w", err)
	}
	return string(b), nil
}

func (s *Store) addActivityTx(tx *sql.Tx, taskID, projectID, boardID, actorID *int64, kind, message string, data any) error {
	payload, err := marshalActivityData(data)
	if err != nil {
		return err
	}
	ts := now()
	_, err = tx.Exec(
		`INSERT INTO activity (task_id, project_id, board_id, actor_id, kind, message, data, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		taskID, projectID, boardID, actorID, kind, message, payload, ts,
	)
	return err
}

func (s *Store) AddActivity(taskID, projectID, boardID, actorID *int64, kind, message string, data any) (*model.Activity, error) {
	payload, err := marshalActivityData(data)
	if err != nil {
		return nil, err
	}
	ts := now()
	res, err := s.db.Exec(
		`INSERT INTO activity (task_id, project_id, board_id, actor_id, kind, message, data, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		taskID, projectID, boardID, actorID, kind, message, payload, ts,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetActivity(id)
}

func (s *Store) GetActivity(id int64) (*model.Activity, error) {
	const q = `
SELECT a.id, a.task_id, a.project_id, a.board_id, a.actor_id, a.kind, a.message, a.data, a.created_at,
       COALESCE(d.username, '')
FROM activity a
LEFT JOIN developers d ON d.id = a.actor_id
WHERE a.id = ?`
	var a model.Activity
	var taskID, projectID, boardID, actorID sql.NullInt64
	var data sql.NullString
	var created, actor string
	err := s.db.QueryRow(q, id).Scan(
		&a.ID, &taskID, &projectID, &boardID, &actorID, &a.Kind, &a.Message, &data, &created, &actor,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.TaskID = int64Ptr(taskID)
	a.ProjectID = int64Ptr(projectID)
	a.BoardID = int64Ptr(boardID)
	a.ActorID = int64Ptr(actorID)
	if data.Valid && data.String != "" {
		a.Data = json.RawMessage(data.String)
	}
	a.CreatedAt = parseTime(created)
	a.ActorRef = actor
	return &a, nil
}

type ActivityFilter struct {
	TaskID     int64
	BoardRef   string
	ProjectRef string
	Limit      int
}

func (s *Store) ListActivity(f ActivityFilter) ([]model.Activity, error) {
	var (
		conds []string
		args  []any
	)
	if f.TaskID > 0 {
		conds = append(conds, "a.task_id = ?")
		args = append(args, f.TaskID)
	}
	if f.BoardRef != "" {
		b, err := s.ResolveBoard(f.BoardRef)
		if err != nil {
			return nil, err
		}
		conds = append(conds, "a.board_id = ?")
		args = append(args, b.ID)
	}
	if f.ProjectRef != "" {
		p, err := s.ResolveProject(f.ProjectRef)
		if err != nil {
			return nil, err
		}
		conds = append(conds, "a.project_id = ?")
		args = append(args, p.ID)
	}

	q := `
SELECT a.id, a.task_id, a.project_id, a.board_id, a.actor_id, a.kind, a.message, a.data, a.created_at,
       COALESCE(d.username, '')
FROM activity a
LEFT JOIN developers d ON d.id = a.actor_id`
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY a.created_at DESC, a.id DESC"
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	q += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Activity{}
	for rows.Next() {
		var a model.Activity
		var taskID, projectID, boardID, actorID sql.NullInt64
		var data sql.NullString
		var created, actor string
		if err := rows.Scan(
			&a.ID, &taskID, &projectID, &boardID, &actorID, &a.Kind, &a.Message, &data, &created, &actor,
		); err != nil {
			return nil, err
		}
		a.TaskID = int64Ptr(taskID)
		a.ProjectID = int64Ptr(projectID)
		a.BoardID = int64Ptr(boardID)
		a.ActorID = int64Ptr(actorID)
		if data.Valid && data.String != "" {
			a.Data = json.RawMessage(data.String)
		}
		a.CreatedAt = parseTime(created)
		a.ActorRef = actor
		out = append(out, a)
	}
	return out, rows.Err()
}
