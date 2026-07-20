package store

import (
	"database/sql"
	"fmt"
	"strings"

	"obdurate/internal/model"
)

type TaskSearchFilter struct {
	Query      string
	BoardRef   string
	ProjectRef string
	Limit      int
}

// isFTSQueryError reports whether err came from an invalid FTS5 MATCH
// expression, as opposed to an unrelated database error. Kept as a defense
// in depth even though buildMatchQuery quotes every term (see below), which
// rules out syntax errors for any input reachable through it today.
func isFTSQueryError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "fts5:") || strings.Contains(msg, "unterminated string")
}

// buildMatchQuery turns free-text user input into an FTS5 MATCH expression:
// each whitespace-separated term is individually double-quoted (internal
// quotes escaped by doubling, FTS5's own escape rule), and the quoted terms
// are combined with FTS5's default implicit AND. Quoting every term makes
// hyphens, colons, parens, and other characters that are normally FTS5
// query-syntax operators (NOT, column filters, grouping, ...) match
// literally instead. Without this, an entirely ordinary search like
// "PROJ-123" (exactly the shape of values tracked under the task metadata
// `pr` key) throws a confusing "no such column: 123" parse error, because
// FTS5 reads the bare hyphen as the NOT operator. The trade-off is that
// FTS5's power-user boolean/prefix syntax isn't exposed to callers — every
// term is a literal, and multi-word input is an AND of literals.
func buildMatchQuery(raw string) string {
	fields := strings.Fields(raw)
	quoted := make([]string, 0, len(fields))
	for _, f := range fields {
		quoted = append(quoted, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " ")
}

// SearchTasks runs a full-text search over task title and description
// (SQLite FTS5, see schema.sql's tasks_fts table), returning hits ordered
// best-match first with bm25 rank and "**"-wrapped highlighted excerpts.
func (s *Store) SearchTasks(f TaskSearchFilter) ([]model.TaskSearchHit, error) {
	query := strings.TrimSpace(f.Query)
	if query == "" {
		return nil, fmt.Errorf("%w: search query is required", ErrInvalidInput)
	}

	conds := []string{"tasks_fts MATCH ?"}
	args := []any{buildMatchQuery(query)}
	if f.BoardRef != "" {
		b, err := s.ResolveBoard(f.BoardRef)
		if err != nil {
			return nil, err
		}
		conds = append(conds, "t.board_id = ?")
		args = append(args, b.ID)
	} else if f.ProjectRef != "" {
		p, err := s.ResolveProject(f.ProjectRef)
		if err != nil {
			return nil, err
		}
		conds = append(conds, "bd.project_id = ?")
		args = append(args, p.ID)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	q := fmt.Sprintf(`
SELECT t.id, t.board_id, t.column_id, t.title, t.description, t.assignee_id, t.priority, t.position,
       t.created_at, t.updated_at, c.name, COALESCE(d.username, ''),
       bm25(tasks_fts) AS rank,
       highlight(tasks_fts, 0, '**', '**') AS title_hl,
       highlight(tasks_fts, 1, '**', '**') AS desc_hl
FROM tasks_fts
JOIN tasks t ON t.id = tasks_fts.rowid
JOIN columns c ON c.id = t.column_id
JOIN boards bd ON bd.id = t.board_id
LEFT JOIN developers d ON d.id = t.assignee_id
WHERE %s
ORDER BY rank
LIMIT %d`, strings.Join(conds, " AND "), limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		if isFTSQueryError(err) {
			return nil, fmt.Errorf("%w: invalid search query %q: %v", ErrInvalidInput, query, err)
		}
		return nil, err
	}

	out := []model.TaskSearchHit{}
	for rows.Next() {
		var hit model.TaskSearchHit
		var assignee sql.NullInt64
		var created, updated, colName, assigneeUser, prio string
		var titleHL, descHL sql.NullString
		if err := rows.Scan(
			&hit.ID, &hit.BoardID, &hit.ColumnID, &hit.Title, &hit.Description, &assignee, &prio, &hit.Position,
			&created, &updated, &colName, &assigneeUser,
			&hit.Rank, &titleHL, &descHL,
		); err != nil {
			rows.Close()
			return nil, err
		}
		hit.AssigneeID = int64Ptr(assignee)
		hit.Priority = model.Priority(prio)
		hit.CreatedAt = parseTime(created)
		hit.UpdatedAt = parseTime(updated)
		hit.ColumnName = colName
		hit.AssigneeRef = assigneeUser
		hit.TitleHighlight = titleHL.String
		hit.DescriptionHighlight = descHL.String
		out = append(out, hit)
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
		meta, err := s.taskMetadata(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Metadata = meta
	}
	return out, nil
}
