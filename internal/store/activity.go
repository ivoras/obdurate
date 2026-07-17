package store

import (
	"database/sql"
	"fmt"
	"strings"

	"obdurate/internal/model"
)

func (s *Store) addActivityTx(tx *sql.Tx, taskID, projectID, boardID, actorID *int64, kind, message string) error {
	ts := now()
	_, err := tx.Exec(
		`INSERT INTO activity (task_id, project_id, board_id, actor_id, kind, message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		taskID, projectID, boardID, actorID, kind, message, ts,
	)
	return err
}

func (s *Store) AddActivity(taskID, projectID, boardID, actorID *int64, kind, message string) (*model.Activity, error) {
	ts := now()
	res, err := s.db.Exec(
		`INSERT INTO activity (task_id, project_id, board_id, actor_id, kind, message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		taskID, projectID, boardID, actorID, kind, message, ts,
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
SELECT a.id, a.task_id, a.project_id, a.board_id, a.actor_id, a.kind, a.message, a.created_at,
       COALESCE(d.username, '')
FROM activity a
LEFT JOIN developers d ON d.id = a.actor_id
WHERE a.id = ?`
	var a model.Activity
	var taskID, projectID, boardID, actorID sql.NullInt64
	var created, actor string
	err := s.db.QueryRow(q, id).Scan(
		&a.ID, &taskID, &projectID, &boardID, &actorID, &a.Kind, &a.Message, &created, &actor,
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
SELECT a.id, a.task_id, a.project_id, a.board_id, a.actor_id, a.kind, a.message, a.created_at,
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

	var out []model.Activity
	for rows.Next() {
		var a model.Activity
		var taskID, projectID, boardID, actorID sql.NullInt64
		var created, actor string
		if err := rows.Scan(
			&a.ID, &taskID, &projectID, &boardID, &actorID, &a.Kind, &a.Message, &created, &actor,
		); err != nil {
			return nil, err
		}
		a.TaskID = int64Ptr(taskID)
		a.ProjectID = int64Ptr(projectID)
		a.BoardID = int64Ptr(boardID)
		a.ActorID = int64Ptr(actorID)
		a.CreatedAt = parseTime(created)
		a.ActorRef = actor
		out = append(out, a)
	}
	return out, rows.Err()
}
