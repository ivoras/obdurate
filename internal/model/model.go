package model

import (
	"encoding/json"
	"time"
)

type Role string

func (r Role) String() string { return string(r) }

const (
	RoleAdmin     Role = "admin"
	RoleLead      Role = "lead"
	RoleDeveloper Role = "developer"
	RoleViewer    Role = "viewer"
)

func ValidRole(r string) bool {
	switch Role(r) {
	case RoleAdmin, RoleLead, RoleDeveloper, RoleViewer:
		return true
	default:
		return false
	}
}

type Priority string

func (p Priority) String() string { return string(p) }

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

func ValidPriority(p string) bool {
	switch Priority(p) {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityCritical:
		return true
	default:
		return false
	}
}

type Developer struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	SlackID   *string   `json:"slack_id,omitempty"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Project struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Board struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"project_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Column struct {
	ID        int64     `json:"id"`
	BoardID   int64     `json:"board_id"`
	Name      string    `json:"name"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Task struct {
	ID          int64     `json:"id"`
	BoardID     int64     `json:"board_id"`
	ColumnID    int64     `json:"column_id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	AssigneeID  *int64    `json:"assignee_id,omitempty"`
	Priority    Priority  `json:"priority"`
	Position    int       `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// hydrated fields
	ColumnName  string            `json:"column_name,omitempty"`
	AssigneeRef string            `json:"assignee,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	WatcherRefs []string          `json:"watchers,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type Activity struct {
	ID        int64  `json:"id"`
	TaskID    *int64 `json:"task_id,omitempty"`
	ProjectID *int64 `json:"project_id,omitempty"`
	BoardID   *int64 `json:"board_id,omitempty"`
	ActorID   *int64 `json:"actor_id,omitempty"`
	Kind      string `json:"kind"`
	Message   string `json:"message"`
	// Data holds a structured JSON payload describing the change
	// (old/new values, snapshots) so state can be reconstructed.
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	// hydrated
	ActorRef string `json:"actor,omitempty"`
}

const (
	ActivityCreated   = "created"
	ActivityUpdated   = "updated"
	ActivityMoved     = "moved"
	ActivityCommented = "commented"
	ActivityWatched   = "watched"
	ActivityUnwatched = "unwatched"
	ActivityAssigned  = "assigned"
	ActivityDeleted   = "deleted"
)

// BoardView is a kanban board snapshot grouped by column.
type BoardView struct {
	Board   Board             `json:"board"`
	Columns []ColumnWithTasks `json:"columns"`
}

type ColumnWithTasks struct {
	Column Column `json:"column"`
	Tasks  []Task `json:"tasks"`
}
