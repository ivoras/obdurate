# Obdurate (`obd`)

CLI project management tool with a kanban-style workflow. Data is stored in SQLite. There is no authentication — any user can perform any action. Developers are identified by id, email, username, or Slack id at the point of use.

## Features

- Multiple **projects**, each with multiple **kanban boards**
- Columns customizable per board (defaults: **Todo / Doing / Done**)
- **Tasks** with title, description, assignee, priority, tags, watchers
- Unified **activity stream** (system events + comments)
- Script-friendly **JSON / CSV / TOON** output and stable process exit codes

## Requirements

- Go 1.22+ (module tested with Go 1.26)
- No CGO required (pure-Go SQLite driver: `modernc.org/sqlite`)

## Build

```bash
# binary in repo root
make build
# or
go build -o obd ./cmd/obd

# install to ~/.local/bin (override with DESTDIR=...)
make install
```

Other targets: `make test`, `make vet`, `make fmt`, `make clean`.

## Quick start

```bash
# create people
./obd developer create --name "Alice" --email alice@example.com --username alice --role lead
./obd developer create --name "Bob" --email bob@example.com --username bob

# project + board (board gets Todo / Doing / Done)
./obd project create --name widget --description "Widget product"
./obd board create --project widget --name sprint-1

# tasks
./obd task create --board widget/sprint-1 --title "Wireframe login" \
  --assignee alice --priority high --tags "ui,auth" --watchers bob --by alice

./obd task move 1 --column Doing --by alice
./obd task comment 1 --by bob --message "Looks good — need API stub"

# views
./obd board show widget/sprint-1
./obd task mine --assignee alice
./obd activity --board widget/sprint-1
./obd export tasks --board widget/sprint-1 --json
```

## Global flags

| Flag | Description |
|------|-------------|
| `--db PATH` | SQLite database path (default: `./db/obdurate.db`; parent dirs are created) |
| `--json` | Machine-readable JSON output |
| `--csv` | Machine-readable CSV output |
| `--toon` | Machine-readable [TOON](https://github.com/toon-format/toon-go) output |
| `-h, --help` | Help |

Use only one of `--json`, `--csv`, or `--toon`.

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `2` | Not found |
| `3` | Invalid input / conflict / already exists |
| `1` | Other error |

## Conceptual model

```
Project  ──┐
           ├── Board ── Column ── Task
           └── Board ── ...
Developer (global) ← assigned to / watches tasks
Activity (unified stream of events + comments)
```

- **Developer roles** (informational only): `admin`, `lead`, `developer`, `viewer`
- **Task priority**: `low`, `medium` (default), `high`, `critical`
- **Developer references**: numeric id, email, username, or Slack id
- **Board references**: numeric id, unique name, or `project/board` (recommended)
- **Column references**: name or id (scoped to a board)

## Commands

### `obd developer` (aliases: `dev`, `user`)

| Command | Description |
|---------|-------------|
| `developer create` | Create a developer |
| `developer list` | List developers |
| `developer get <ref>` | Get by id, email, username, or slack-id |
| `developer update <ref>` | Update fields |
| `developer delete <ref>` | Delete developer |

**create flags:** `--name` (req), `--email` (req), `--username` (req), `--slack-id`, `--role` (default `developer`)

**update flags:** `--name`, `--email`, `--username`, `--slack-id`, `--clear-slack-id`, `--role`

### `obd project` (alias: `proj`)

| Command | Description |
|---------|-------------|
| `project create` | Create project (`--name`, `--description`) |
| `project list` | List projects |
| `project get <ref>` | Get by id or name |
| `project update <ref>` | Update (`--name`, `--description`) |
| `project delete <ref>` | Delete project and all boards/tasks |

### `obd board`

| Command | Description |
|---------|-------------|
| `board create` | Create board (`--project`, `--name`, `--description`); seeds Todo/Doing/Done |
| `board list` | List boards (`--project` optional filter) |
| `board get <ref>` | Get by id, name, or `project/name` |
| `board update <ref>` | Update (`--name`, `--description`) |
| `board delete <ref>` | Delete board |
| `board show <ref>` | Kanban view grouped by column |

### `obd column` (alias: `col`)

| Command | Description |
|---------|-------------|
| `column add` | Add column (`--board`, `--name`, `--position`) |
| `column list` | List columns (`--board`) |
| `column rename <column>` | Rename (`--board`, `--name`) |
| `column reorder <column>` | Reorder (`--board`, `--position`) |
| `column delete <column>` | Delete empty column (`--board`) |

### `obd task`

| Command | Description |
|---------|-------------|
| `task create` | Create task on a board |
| `task list` | List with filters |
| `task get <id>` | Get by id |
| `task update <id>` | Update fields |
| `task move <id>` | Move to another column |
| `task delete <id>` | Delete task |
| `task comment <id>` | Add comment to activity stream |
| `task watch <id>` | Add watcher (`--by`) |
| `task unwatch <id>` | Remove watcher (`--by`) |
| `task activity <id>` | Show task activity/comments |
| `task mine` | Tasks assigned to a developer (`--assignee`) |

**create flags:** `--board` (req), `--title` (req), `--description`, `--column`, `--assignee`, `--priority`, `--tags` (comma-separated), `--watchers` (comma-separated refs), `--by` (actor for activity)

**list flags:** `--board`, `--project`, `--assignee`, `--column` (with board), `--watcher`, `--tag`

**update flags:** `--title`, `--description`, `--assignee`, `--clear-assignee`, `--priority`, `--tags` (replace), `--by`

**move flags:** `--column` (req), `--position`, `--by`

### `obd activity`

List activity / comments globally with filters:

- `--board`, `--project`, `--task`, `--limit` (default 50)

### `obd export`

| Command | Description |
|---------|-------------|
| `export tasks` | Export tasks (`--board` or `--project`); defaults to JSON if no `--json`/`--csv`/`--toon` |

### `obd version`

Print version string.

### `obd completion`

Generate shell completion scripts (`obd completion --help`).

## Examples

```bash
# custom column between Doing and Done
./obd column add --board widget/sprint-1 --name Review --position 2

# my work as bob (ref by email)
./obd task mine --assignee bob@example.com

# JSON for scripting
./obd task list --board widget/sprint-1 --json | jq '.[].title'

# TOON for compact structured output
./obd task list --board widget/sprint-1 --toon

# CSV export
./obd export tasks --project widget --csv > tasks.csv
```

## License

Use and modify as needed for your environment.
