# obd command reference

Complete flag reference for the `obd` CLI. Every command accepts the global
flags first: `--db PATH` (SQLite file; parent dirs auto-created) and exactly
one optional output format: `--json`, `--csv`, or `--toon` (default:
human-readable table).

Exit codes: `0` OK · `2` not found · `3` invalid input / already exists /
conflict · `1` other.

## Reference forms

- **Developer ref**: numeric id | email | username | Slack id (case-insensitive).
- **Project ref**: numeric id | name (case-insensitive).
- **Board ref**: numeric id | `project/board` (preferred) | bare board name
  (errors with "ambiguous" if the name exists in several projects).
- **Column ref**: name (case-insensitive) or numeric id, scoped to its board.
- Numeric strings are tried as ids first; an entity literally named "123" is
  shadowed by id 123.

## developer (aliases: dev, user)

| Command | Flags |
|---|---|
| `developer create` | `--name` (req), `--email` (req), `--username` (req), `--slack-id`, `--role admin\|lead\|developer\|viewer` (default `developer`) |
| `developer list` | — |
| `developer get <ref>` | — |
| `developer update <ref>` | `--name`, `--email`, `--username`, `--slack-id`, `--clear-slack-id`, `--role` — only provided flags change; empty name/email/username are rejected |
| `developer delete <ref>` | — (tasks assigned to them become unassigned; their watches disappear) |

Email and username are unique (case-insensitive). Roles are informational
only — they grant or restrict nothing.

A brand-new database is auto-seeded with a project `default` containing a
board `main` — the fallback target for tasks created without an explicit
project. It is ordinary and only re-seeded when zero projects exist.

**Slug rule** (projects and boards): names are lowercase ASCII slugs —
letters, digits, `-` or `_`, must start and end with a letter or digit, max
64 chars. Uppercase input is lowercased automatically; anything else (spaces,
`/`, unicode) is rejected with exit 3. Column names and task titles are
free-form.

## project (alias: proj)

| Command | Flags |
|---|---|
| `project create` | `--name` (req, unique, slug), `--description` |
| `project list` | — |
| `project get <ref>` | — |
| `project update <ref>` | `--name`, `--description` |
| `project delete <ref>` | — deletes ALL its boards and tasks (cascades) |

## board

| Command | Flags |
|---|---|
| `board create` | `--project` (req), `--name` (req, unique within project, slug), `--description`. Seeds columns Todo, Doing, Done |
| `board list` | `--project` (optional filter) |
| `board get <ref>` | — |
| `board update <ref>` | `--name`, `--description` |
| `board delete <ref>` | — deletes all its tasks (cascades) |
| `board show <ref>` | kanban view grouped by column; with `--json` returns `{board, columns: [{column, tasks}]}` |

## column (alias: col)

| Command | Flags |
|---|---|
| `column add` | `--board` (req), `--name` (req, unique per board), `--position` (0-based; omitted = append; clamped to valid range) |
| `column list` | `--board` (req) |
| `column rename <column>` | `--board` (req), `--name` (req) |
| `column reorder <column>` | `--board` (req), `--position` (req, clamped) |
| `column delete <column>` | `--board` (req); refuses (exit 3) if the column still contains tasks |

## task

| Command | Flags |
|---|---|
| `task create` | `--board` (req), `--title` (req), `--description`, `--column` (default: first column), `--assignee`, `--priority low\|medium\|high\|critical` (default `medium`), `--tags` (comma-separated), `--watchers` (comma-separated developer refs), `--by` (actor) |
| `task list` | `--board`, `--project`, `--assignee`, `--column` (only honored together with `--board`), `--watcher`, `--tag` — all optional, combined with AND |
| `task get <id>` | — (id is the only accepted task reference) |
| `task update <id>` | `--title`, `--description`, `--assignee`, `--clear-assignee`, `--priority`, `--tags` (REPLACES the full list), `--by` |
| `task move <id>` | `--column` (req), `--position` (0-based within column, clamped), `--by` |
| `task delete <id>` | `--by`. Permanent; history is preserved in activity (see below) |
| `task comment <id>` | `--message` (req), `--by` |
| `task watch <id>` / `task unwatch <id>` | `--by` (req) |
| `task activity <id>` | `--limit` (default 50) |
| `task mine` | `--assignee` (req) — tasks assigned to that developer across all projects |

Task JSON fields: `id`, `board_id`, `column_id`, `column_name`, `title`,
`description`, `priority`, `position`, `assignee` (username), `assignee_id`,
`tags`, `watchers`, `created_at`, `updated_at`.

## activity

`activity [--board REF] [--project REF] [--task ID] [--limit N]` — unified
stream, newest first (default limit 50, max 1000).

Activity JSON fields: `id`, `task_id`, `project_id`, `board_id`, `actor_id`,
`actor` (username), `kind`, `message`, `data`, `created_at`.

### data payloads by kind

| kind | data |
|---|---|
| `created` | `{"task": <snapshot>}` |
| `updated` | `{"changes": {"<field>": {"old": ..., "new": ...}}}` — fields: `title`, `description`, `priority`, `assignee` (username or null), `tags` (arrays) |
| `moved` | `{"from": {"column", "column_id", "position"}, "to": {same}}` |
| `deleted` | `{"task": <snapshot of final state>}` |
| `watched` / `unwatched` | `{"developer": "<username>"}` |
| `commented` | none — `message` is the comment text |

Snapshot = `{"id", "title", "description", "column", "column_id",
"priority", "position", "assignee" (username or null), "tags", "watchers"}`.

When a task is deleted, its activity rows stay in the board/project streams
with `task_id` set to null and the original id preserved as `data.task_id`,
so `activity --task <id>` no longer finds them but `activity --board ...`
does.

## export

`export tasks (--board REF | --project REF)` — like `task list`, but defaults
to JSON when no format flag is given (for scripting).

## Misc

- `version` — print version; `completion <shell>` — shell completions.
  Neither touches the database.
- Timestamps are UTC RFC3339.
- Two processes can share one database safely (SQLite busy timeout 5 s).
