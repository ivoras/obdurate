PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS developers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL,
    email      TEXT    NOT NULL COLLATE NOCASE,
    username   TEXT    NOT NULL COLLATE NOCASE,
    slack_id   TEXT             COLLATE NOCASE,
    role       TEXT    NOT NULL CHECK (role IN ('admin', 'lead', 'developer', 'viewer')),
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL,
    UNIQUE (email),
    UNIQUE (username)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_developers_slack_id
    ON developers(slack_id) WHERE slack_id IS NOT NULL AND slack_id != '';

CREATE TABLE IF NOT EXISTS projects (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL COLLATE NOCASE,
    description TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL,
    UNIQUE (name)
);

CREATE TABLE IF NOT EXISTS boards (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT    NOT NULL COLLATE NOCASE,
    description TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL,
    UNIQUE (project_id, name)
);

CREATE TABLE IF NOT EXISTS columns (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    board_id   INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL COLLATE NOCASE,
    position   INTEGER NOT NULL,
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL,
    UNIQUE (board_id, name),
    UNIQUE (board_id, position)
);

CREATE TABLE IF NOT EXISTS tasks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    board_id    INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    column_id   INTEGER NOT NULL REFERENCES columns(id) ON DELETE RESTRICT,
    title       TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    assignee_id INTEGER REFERENCES developers(id) ON DELETE SET NULL,
    priority    TEXT    NOT NULL DEFAULT 'medium'
                CHECK (priority IN ('low', 'medium', 'high', 'critical')),
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_board ON tasks(board_id);
CREATE INDEX IF NOT EXISTS idx_tasks_column ON tasks(column_id);
CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasks(assignee_id);

CREATE TABLE IF NOT EXISTS tags (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT    NOT NULL COLLATE NOCASE UNIQUE
);

CREATE TABLE IF NOT EXISTS task_tags (
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, tag_id)
);

CREATE TABLE IF NOT EXISTS task_watchers (
    task_id      INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    developer_id INTEGER NOT NULL REFERENCES developers(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, developer_id)
);

CREATE TABLE IF NOT EXISTS activity (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id    INTEGER REFERENCES tasks(id) ON DELETE CASCADE,
    project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
    board_id   INTEGER REFERENCES boards(id) ON DELETE CASCADE,
    actor_id   INTEGER REFERENCES developers(id) ON DELETE SET NULL,
    kind       TEXT    NOT NULL,
    message    TEXT    NOT NULL,
    created_at TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_activity_task ON activity(task_id);
CREATE INDEX IF NOT EXISTS idx_activity_board ON activity(board_id);
CREATE INDEX IF NOT EXISTS idx_activity_project ON activity(project_id);
CREATE INDEX IF NOT EXISTS idx_activity_created ON activity(created_at);
