package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"obdurate/internal/model"
)

type TaskCreate struct {
	BoardRef    string
	ColumnRef   string // optional; default first column
	Title       string
	Description string
	AssigneeRef string
	Priority    model.Priority
	Tags        []string
	WatcherRefs []string
	ActorRef    string
}

func (s *Store) CreateTask(in TaskCreate) (*model.Task, error) {
	b, err := s.ResolveBoard(in.BoardRef)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	priority := in.Priority
	if priority == "" {
		priority = model.PriorityMedium
	}
	if !model.ValidPriority(string(priority)) {
		return nil, fmt.Errorf("%w: invalid priority %q", ErrInvalidInput, priority)
	}

	var col *model.Column
	if strings.TrimSpace(in.ColumnRef) == "" {
		cols, err := s.ListColumns(b.ID)
		if err != nil {
			return nil, err
		}
		if len(cols) == 0 {
			return nil, fmt.Errorf("%w: board has no columns", ErrConflict)
		}
		col = &cols[0]
	} else {
		col, err = s.ResolveColumn(b.ID, in.ColumnRef)
		if err != nil {
			return nil, err
		}
	}

	var assigneeID *int64
	var assigneeUser string
	if strings.TrimSpace(in.AssigneeRef) != "" {
		d, err := s.ResolveDeveloper(in.AssigneeRef)
		if err != nil {
			return nil, fmt.Errorf("assignee: %w", err)
		}
		assigneeID = &d.ID
		assigneeUser = d.Username
	}

	var actorID *int64
	if strings.TrimSpace(in.ActorRef) != "" {
		a, err := s.ResolveDeveloper(in.ActorRef)
		if err != nil {
			return nil, fmt.Errorf("actor: %w", err)
		}
		actorID = &a.ID
	}

	watchers, err := s.resolveDevelopers(in.WatcherRefs)
	if err != nil {
		return nil, err
	}
	watcherIDs := make([]int64, 0, len(watchers))
	watcherUsers := make([]string, 0, len(watchers))
	for _, w := range watchers {
		watcherIDs = append(watcherIDs, w.ID)
		watcherUsers = append(watcherUsers, w.Username)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var maxPos sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(position) FROM tasks WHERE column_id = ?`, col.ID).Scan(&maxPos); err != nil {
		return nil, err
	}
	pos := 0
	if maxPos.Valid {
		pos = int(maxPos.Int64) + 1
	}

	ts := now()
	res, err := tx.Exec(
		`INSERT INTO tasks (board_id, column_id, title, description, assignee_id, priority, position, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, col.ID, title, in.Description, assigneeID, string(priority), pos, ts, ts,
	)
	if err != nil {
		return nil, err
	}
	taskID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	appliedTags, err := s.setTagsTx(tx, taskID, in.Tags)
	if err != nil {
		return nil, err
	}
	if err := s.setWatcherIDsTx(tx, taskID, watcherIDs); err != nil {
		return nil, err
	}

	var assigneeVal any
	if assigneeUser != "" {
		assigneeVal = assigneeUser
	}
	data := map[string]any{"task": map[string]any{
		"id":          taskID,
		"title":       title,
		"description": in.Description,
		"column":      col.Name,
		"column_id":   col.ID,
		"priority":    string(priority),
		"position":    pos,
		"assignee":    assigneeVal,
		"tags":        appliedTags,
		"watchers":    watcherUsers,
	}}
	msg := fmt.Sprintf("created task %q in column %q", title, col.Name)
	if err := s.addActivityTx(tx, &taskID, &b.ProjectID, &b.ID, actorID, model.ActivityCreated, msg, data); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetTask(taskID)
}

func (s *Store) GetTask(id int64) (*model.Task, error) {
	const q = `
SELECT t.id, t.board_id, t.column_id, t.title, t.description, t.assignee_id, t.priority, t.position,
       t.created_at, t.updated_at, c.name, COALESCE(d.username, '')
FROM tasks t
JOIN columns c ON c.id = t.column_id
LEFT JOIN developers d ON d.id = t.assignee_id
WHERE t.id = ?`
	var t model.Task
	var assignee sql.NullInt64
	var created, updated, colName, assigneeUser string
	var prio string
	err := s.db.QueryRow(q, id).Scan(
		&t.ID, &t.BoardID, &t.ColumnID, &t.Title, &t.Description, &assignee, &prio, &t.Position,
		&created, &updated, &colName, &assigneeUser,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.AssigneeID = int64Ptr(assignee)
	t.Priority = model.Priority(prio)
	t.CreatedAt = parseTime(created)
	t.UpdatedAt = parseTime(updated)
	t.ColumnName = colName
	t.AssigneeRef = assigneeUser
	tags, err := s.taskTags(id)
	if err != nil {
		return nil, err
	}
	t.Tags = tags
	watchers, err := s.taskWatchers(id)
	if err != nil {
		return nil, err
	}
	t.WatcherRefs = watchers
	return &t, nil
}

func (s *Store) taskTags(taskID int64) ([]string, error) {
	rows, err := s.db.Query(`
SELECT tg.name FROM tags tg
JOIN task_tags tt ON tt.tag_id = tg.id
WHERE tt.task_id = ? ORDER BY tg.name`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		tags = append(tags, n)
	}
	return tags, rows.Err()
}

func (s *Store) taskWatchers(taskID int64) ([]string, error) {
	rows, err := s.db.Query(`
SELECT d.username FROM developers d
JOIN task_watchers tw ON tw.developer_id = d.id
WHERE tw.task_id = ? ORDER BY d.username`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// setTagsTx replaces a task's tags and returns the normalized applied list.
func (s *Store) setTagsTx(tx *sql.Tx, taskID int64, tags []string) ([]string, error) {
	if _, err := tx.Exec(`DELETE FROM task_tags WHERE task_id = ?`, taskID); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	applied := []string{}
	for _, raw := range tags {
		name := strings.TrimSpace(raw)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		var tagID int64
		err := tx.QueryRow(`SELECT id FROM tags WHERE lower(name) = lower(?)`, name).Scan(&tagID)
		if err == sql.ErrNoRows {
			res, err := tx.Exec(`INSERT INTO tags (name) VALUES (?)`, name)
			if err != nil {
				return nil, err
			}
			tagID, err = res.LastInsertId()
			if err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`INSERT INTO task_tags (task_id, tag_id) VALUES (?, ?)`, taskID, tagID); err != nil {
			return nil, err
		}
		applied = append(applied, name)
	}
	return applied, nil
}

func (s *Store) resolveDevelopers(refs []string) ([]model.Developer, error) {
	seen := map[int64]bool{}
	var out []model.Developer
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		d, err := s.ResolveDeveloper(ref)
		if err != nil {
			return nil, fmt.Errorf("watcher %q: %w", ref, err)
		}
		if seen[d.ID] {
			continue
		}
		seen[d.ID] = true
		out = append(out, *d)
	}
	return out, nil
}

// sameTagSet compares tag lists ignoring order and case.
func sameTagSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, t := range a {
		set[strings.ToLower(t)] = true
	}
	for _, t := range b {
		if !set[strings.ToLower(t)] {
			return false
		}
	}
	return true
}

func (s *Store) setWatcherIDsTx(tx *sql.Tx, taskID int64, ids []int64) error {
	if _, err := tx.Exec(`DELETE FROM task_watchers WHERE task_id = ?`, taskID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.Exec(`INSERT INTO task_watchers (task_id, developer_id) VALUES (?, ?)`, taskID, id); err != nil {
			return err
		}
	}
	return nil
}

type TaskUpdate struct {
	Title       *string
	Description *string
	AssigneeRef *string // empty clears
	Priority    *model.Priority
	Tags        *[]string
	ActorRef    string
}

func (s *Store) UpdateTask(id int64, u TaskUpdate) (*model.Task, error) {
	t, err := s.GetTask(id)
	if err != nil {
		return nil, err
	}
	b, err := s.GetBoard(t.BoardID)
	if err != nil {
		return nil, err
	}

	var actorID *int64
	if strings.TrimSpace(u.ActorRef) != "" {
		a, err := s.ResolveDeveloper(u.ActorRef)
		if err != nil {
			return nil, fmt.Errorf("actor: %w", err)
		}
		actorID = &a.ID
	}

	var changes []string
	changed := map[string]any{}
	if u.Title != nil {
		title := strings.TrimSpace(*u.Title)
		if title == "" {
			return nil, fmt.Errorf("%w: title cannot be empty", ErrInvalidInput)
		}
		if title != t.Title {
			changes = append(changes, fmt.Sprintf("title %q → %q", t.Title, title))
			changed["title"] = fieldChange(t.Title, title)
			t.Title = title
		}
	}
	if u.Description != nil && *u.Description != t.Description {
		changes = append(changes, "description updated")
		changed["description"] = fieldChange(t.Description, *u.Description)
		t.Description = *u.Description
	}
	if u.Priority != nil {
		if !model.ValidPriority(string(*u.Priority)) {
			return nil, fmt.Errorf("%w: invalid priority %q", ErrInvalidInput, *u.Priority)
		}
		if *u.Priority != t.Priority {
			changes = append(changes, fmt.Sprintf("priority %s → %s", t.Priority, *u.Priority))
			changed["priority"] = fieldChange(string(t.Priority), string(*u.Priority))
			t.Priority = *u.Priority
		}
	}
	if u.AssigneeRef != nil {
		var oldAssignee any
		if t.AssigneeRef != "" {
			oldAssignee = t.AssigneeRef
		}
		if strings.TrimSpace(*u.AssigneeRef) == "" {
			if t.AssigneeID != nil {
				changes = append(changes, "assignee cleared")
				changed["assignee"] = fieldChange(oldAssignee, nil)
				t.AssigneeID = nil
			}
		} else {
			d, err := s.ResolveDeveloper(*u.AssigneeRef)
			if err != nil {
				return nil, fmt.Errorf("assignee: %w", err)
			}
			if t.AssigneeID == nil || *t.AssigneeID != d.ID {
				changes = append(changes, fmt.Sprintf("assignee → %s", d.Username))
				changed["assignee"] = fieldChange(oldAssignee, d.Username)
				t.AssigneeID = &d.ID
			}
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	ts := now()
	_, err = tx.Exec(
		`UPDATE tasks SET title=?, description=?, assignee_id=?, priority=?, updated_at=? WHERE id=?`,
		t.Title, t.Description, t.AssigneeID, string(t.Priority), ts, t.ID,
	)
	if err != nil {
		return nil, err
	}

	if u.Tags != nil {
		oldTags := t.Tags
		if oldTags == nil {
			oldTags = []string{}
		}
		newTags, err := s.setTagsTx(tx, t.ID, *u.Tags)
		if err != nil {
			return nil, err
		}
		if !sameTagSet(oldTags, newTags) {
			changes = append(changes, "tags updated")
			changed["tags"] = fieldChange(oldTags, newTags)
		}
	}

	if len(changes) > 0 {
		msg := "updated: " + strings.Join(changes, "; ")
		data := map[string]any{"changes": changed}
		if err := s.addActivityTx(tx, &t.ID, &b.ProjectID, &b.ID, actorID, model.ActivityUpdated, msg, data); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetTask(t.ID)
}

func (s *Store) MoveTask(id int64, columnRef string, position *int, actorRef string) (*model.Task, error) {
	t, err := s.GetTask(id)
	if err != nil {
		return nil, err
	}
	b, err := s.GetBoard(t.BoardID)
	if err != nil {
		return nil, err
	}
	col, err := s.ResolveColumn(t.BoardID, columnRef)
	if err != nil {
		return nil, err
	}

	var actorID *int64
	if strings.TrimSpace(actorRef) != "" {
		a, err := s.ResolveDeveloper(actorRef)
		if err != nil {
			return nil, fmt.Errorf("actor: %w", err)
		}
		actorID = &a.ID
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Compact source column after move.
	fromColumn := t.ColumnID
	fromName := t.ColumnName
	fromPos := t.Position

	// Remove-then-insert: order the destination column without the moving
	// task, clamp the requested position into [0, len], splice the task in,
	// and rewrite positions 0..n. Handles same-column moves without holes.
	destRows, err := tx.Query(`SELECT id FROM tasks WHERE column_id = ? AND id != ? ORDER BY position, id`, col.ID, t.ID)
	if err != nil {
		return nil, err
	}
	var destIDs []int64
	for destRows.Next() {
		var tid int64
		if err := destRows.Scan(&tid); err != nil {
			destRows.Close()
			return nil, err
		}
		destIDs = append(destIDs, tid)
	}
	if err := destRows.Err(); err != nil {
		destRows.Close()
		return nil, err
	}
	destRows.Close()

	pos := len(destIDs)
	if position != nil {
		pos = *position
		if pos < 0 {
			pos = 0
		}
		if pos > len(destIDs) {
			pos = len(destIDs)
		}
	}
	ordered := append(destIDs[:pos:pos], append([]int64{t.ID}, destIDs[pos:]...)...)

	ts := now()
	_, err = tx.Exec(`UPDATE tasks SET column_id=?, updated_at=? WHERE id=?`, col.ID, ts, t.ID)
	if err != nil {
		return nil, err
	}
	for i, tid := range ordered {
		if _, err := tx.Exec(`UPDATE tasks SET position = ? WHERE id = ?`, i, tid); err != nil {
			return nil, err
		}
	}

	// Compact original column positions.
	if fromColumn != col.ID {
		rows, err := tx.Query(`SELECT id FROM tasks WHERE column_id = ? ORDER BY position, id`, fromColumn)
		if err != nil {
			return nil, err
		}
		var ids []int64
		for rows.Next() {
			var tid int64
			if err := rows.Scan(&tid); err != nil {
				rows.Close()
				return nil, err
			}
			ids = append(ids, tid)
		}
		rows.Close()
		for i, tid := range ids {
			if _, err := tx.Exec(`UPDATE tasks SET position = ? WHERE id = ?`, i, tid); err != nil {
				return nil, err
			}
		}
	}

	data := map[string]any{
		"from": map[string]any{"column": fromName, "column_id": fromColumn, "position": fromPos},
		"to":   map[string]any{"column": col.Name, "column_id": col.ID, "position": pos},
	}
	msg := fmt.Sprintf("moved from %q to %q", fromName, col.Name)
	if err := s.addActivityTx(tx, &t.ID, &b.ProjectID, &b.ID, actorID, model.ActivityMoved, msg, data); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetTask(t.ID)
}

func (s *Store) DeleteTask(id int64, actorRef string) error {
	t, err := s.GetTask(id)
	if err != nil {
		return err
	}
	b, err := s.GetBoard(t.BoardID)
	if err != nil {
		return err
	}
	var actorID *int64
	if strings.TrimSpace(actorRef) != "" {
		a, err := s.ResolveDeveloper(actorRef)
		if err != nil {
			return fmt.Errorf("actor: %w", err)
		}
		actorID = &a.ID
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Log at board level (task_id NULL): activity rows with the task's id
	// would be removed by ON DELETE CASCADE. The data payload carries a full
	// snapshot of the task's final state.
	data := map[string]any{"task": taskSnapshot(t)}
	msg := fmt.Sprintf("deleted task %q (#%d)", t.Title, t.ID)
	if err := s.addActivityTx(tx, nil, &b.ProjectID, &b.ID, actorID, model.ActivityDeleted, msg, data); err != nil {
		return err
	}
	// Preserve the task's activity history, which the task_id cascade would
	// otherwise delete: detach rows by moving the id into the JSON payload.
	if _, err := tx.Exec(
		`UPDATE activity SET data = json_set(COALESCE(data, '{}'), '$.task_id', task_id), task_id = NULL WHERE task_id = ?`,
		t.ID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tasks WHERE id = ?`, t.ID); err != nil {
		return err
	}
	return tx.Commit()
}

type TaskFilter struct {
	BoardRef    string
	ProjectRef  string
	AssigneeRef string
	ColumnRef   string
	// WatcherRef lists tasks the developer watches
	WatcherRef string
	// Tag filter
	Tag string
}

func (s *Store) ListTasks(f TaskFilter) ([]model.Task, error) {
	var (
		conds []string
		args  []any
	)
	if f.BoardRef != "" {
		b, err := s.ResolveBoard(f.BoardRef)
		if err != nil {
			return nil, err
		}
		conds = append(conds, "t.board_id = ?")
		args = append(args, b.ID)
		if f.ColumnRef != "" {
			c, err := s.ResolveColumn(b.ID, f.ColumnRef)
			if err != nil {
				return nil, err
			}
			conds = append(conds, "t.column_id = ?")
			args = append(args, c.ID)
		}
	} else if f.ProjectRef != "" {
		p, err := s.ResolveProject(f.ProjectRef)
		if err != nil {
			return nil, err
		}
		conds = append(conds, "b.project_id = ?")
		args = append(args, p.ID)
	}
	if f.AssigneeRef != "" {
		d, err := s.ResolveDeveloper(f.AssigneeRef)
		if err != nil {
			return nil, err
		}
		conds = append(conds, "t.assignee_id = ?")
		args = append(args, d.ID)
	}
	if f.WatcherRef != "" {
		d, err := s.ResolveDeveloper(f.WatcherRef)
		if err != nil {
			return nil, err
		}
		conds = append(conds, `EXISTS (SELECT 1 FROM task_watchers tw WHERE tw.task_id = t.id AND tw.developer_id = ?)`)
		args = append(args, d.ID)
	}
	if f.Tag != "" {
		conds = append(conds, `EXISTS (
			SELECT 1 FROM task_tags tt JOIN tags tg ON tg.id = tt.tag_id
			WHERE tt.task_id = t.id AND lower(tg.name) = lower(?)
		)`)
		args = append(args, f.Tag)
	}

	q := `
SELECT t.id, t.board_id, t.column_id, t.title, t.description, t.assignee_id, t.priority, t.position,
       t.created_at, t.updated_at, c.name, COALESCE(d.username, '')
FROM tasks t
JOIN columns c ON c.id = t.column_id
JOIN boards b ON b.id = t.board_id
LEFT JOIN developers d ON d.id = t.assignee_id`
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY t.board_id, c.position, t.position, t.id"

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}

	out := []model.Task{}
	for rows.Next() {
		var t model.Task
		var assignee sql.NullInt64
		var created, updated, colName, assigneeUser string
		var prio string
		if err := rows.Scan(
			&t.ID, &t.BoardID, &t.ColumnID, &t.Title, &t.Description, &assignee, &prio, &t.Position,
			&created, &updated, &colName, &assigneeUser,
		); err != nil {
			rows.Close()
			return nil, err
		}
		t.AssigneeID = int64Ptr(assignee)
		t.Priority = model.Priority(prio)
		t.CreatedAt = parseTime(created)
		t.UpdatedAt = parseTime(updated)
		t.ColumnName = colName
		t.AssigneeRef = assigneeUser
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	// Hydrate after closing rows: MaxOpenConns(1) deadlocks on nested queries.
	for i := range out {
		tags, err := s.taskTags(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Tags = tags
		watchers, err := s.taskWatchers(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].WatcherRefs = watchers
	}
	return out, nil
}

func (s *Store) BoardView(boardRef string) (*model.BoardView, error) {
	b, err := s.ResolveBoard(boardRef)
	if err != nil {
		return nil, err
	}
	cols, err := s.ListColumns(b.ID)
	if err != nil {
		return nil, err
	}
	tasks, err := s.ListTasks(TaskFilter{BoardRef: strconv.FormatInt(b.ID, 10)})
	if err != nil {
		return nil, err
	}
	byCol := map[int64][]model.Task{}
	for _, t := range tasks {
		byCol[t.ColumnID] = append(byCol[t.ColumnID], t)
	}
	view := &model.BoardView{Board: *b, Columns: []model.ColumnWithTasks{}}
	for _, c := range cols {
		colTasks := byCol[c.ID]
		if colTasks == nil {
			colTasks = []model.Task{}
		}
		view.Columns = append(view.Columns, model.ColumnWithTasks{
			Column: c,
			Tasks:  colTasks,
		})
	}
	return view, nil
}

func (s *Store) CommentTask(id int64, actorRef, message string) (*model.Activity, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, fmt.Errorf("%w: comment message is required", ErrInvalidInput)
	}
	t, err := s.GetTask(id)
	if err != nil {
		return nil, err
	}
	b, err := s.GetBoard(t.BoardID)
	if err != nil {
		return nil, err
	}
	var actorID *int64
	if strings.TrimSpace(actorRef) != "" {
		a, err := s.ResolveDeveloper(actorRef)
		if err != nil {
			return nil, fmt.Errorf("actor: %w", err)
		}
		actorID = &a.ID
	}
	return s.AddActivity(&t.ID, &b.ProjectID, &b.ID, actorID, model.ActivityCommented, message, nil)
}

func (s *Store) WatchTask(id int64, devRef string) error {
	t, err := s.GetTask(id)
	if err != nil {
		return err
	}
	d, err := s.ResolveDeveloper(devRef)
	if err != nil {
		return err
	}
	b, err := s.GetBoard(t.BoardID)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`INSERT OR IGNORE INTO task_watchers (task_id, developer_id) VALUES (?, ?)`, t.ID, d.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		data := map[string]any{"developer": d.Username}
		msg := fmt.Sprintf("%s is now watching", d.Username)
		if err := s.addActivityTx(tx, &t.ID, &b.ProjectID, &b.ID, &d.ID, model.ActivityWatched, msg, data); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UnwatchTask(id int64, devRef string) error {
	t, err := s.GetTask(id)
	if err != nil {
		return err
	}
	d, err := s.ResolveDeveloper(devRef)
	if err != nil {
		return err
	}
	b, err := s.GetBoard(t.BoardID)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM task_watchers WHERE task_id = ? AND developer_id = ?`, t.ID, d.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		data := map[string]any{"developer": d.Username}
		msg := fmt.Sprintf("%s stopped watching", d.Username)
		if err := s.addActivityTx(tx, &t.ID, &b.ProjectID, &b.ID, &d.ID, model.ActivityUnwatched, msg, data); err != nil {
			return err
		}
	}
	return tx.Commit()
}
