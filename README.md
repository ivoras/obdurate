# Obdurate (`obd`)

CLI project management tool with a kanban-style workflow. Data is stored in SQLite. There is no authentication — any user can perform any action. Developers are identified by id, email, username, or Slack id at the point of use.

## Features

- Multiple **projects**, each with multiple **kanban boards**
- Columns customizable per board (defaults: **Todo / Doing / Done**)
- **Tasks** with title, description, assignee, priority, tags, watchers, metadata (key/value)
- Unified **activity stream** (system events + comments)
- Script-friendly **JSON / CSV / TOON** output and stable process exit codes

## Requirements

- **Go**: track a [current stable](https://go.dev/dl/) release (module is `go 1.26`; `modernc.org/sqlite` requires **1.25+**)
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
./obd task metadata set 1 jira-key PROJ-123 --by alice

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

- **Project and board names are slugs**: lowercase ASCII letters, digits,
  `-` or `_`, starting and ending with a letter or digit, max 64 chars.
  Input is lowercased automatically (`--name Widget` → `widget`); names with
  spaces or other characters are rejected. Put human-readable titles in
  `--description`. (Column names and task titles are free-form.)
- **Task metadata keys are slugs** (same rules as project/board names, e.g.
  `jira-key`); values are free-form strings. Each key is unique per task —
  setting an existing key overwrites its value. Not shown in table output;
  use `--json`/`--toon` or `task metadata list <id>`.
- **Default project**: a brand-new database is seeded with a project named
  `default` containing a board `main` (with the standard columns), so tasks
  can be created immediately without any setup. It is an ordinary project —
  rename or delete it freely; it is only re-seeded when the database has no
  projects at all.
- **Developer roles** (informational only): `admin`, `lead`, `developer`, `viewer`
- **Task priority**: `low`, `medium` (default), `high`, `critical`
- **Developer references**: numeric id, email, username, or Slack id
- **Board references**: numeric id, unique name, or `project/board` (recommended)
- **Column references**: name or id (scoped to a board)
- **Positions**: explicit `--position` values are clamped to the valid range
  (negative → 0, past-the-end → append)
- Machine-readable list output is always a JSON array (`[]` when empty, never `null`)

## Commands

### `obd developer` (aliases: `dev`, `user`)

| Command | Description |
|---------|-------------|
| `developer create` | Create a developer |
| `developer list` | List developers |
| `developer get <ref>` | Get by id, email, username, or slack-id |
| `developer update <ref>` | Update fields |
| `developer delete <ref>` | Delete developer |
| `developer tasks <ref>` | List all tasks assigned to the developer |

**create flags:** `--name` (req), `--email` (req), `--username` (req), `--slack-id`, `--role` (default `developer`)

**update flags:** `--name`, `--email`, `--username`, `--slack-id`, `--clear-slack-id`, `--role`

### `obd project` (alias: `proj`)

| Command | Description |
|---------|-------------|
| `project create` | Create project (`--name`, `--description`, `--by`) |
| `project list` | List projects |
| `project get <ref>` | Get by id or name |
| `project update <ref>` | Update (`--name`, `--description`, `--by`) |
| `project delete <ref>` | Delete project and all boards/tasks (`--by`) |
| `project tasks <ref>` | List all tasks in the project (all its boards) |

### `obd board`

| Command | Description |
|---------|-------------|
| `board create` | Create board (`--project`, `--name`, `--description`, `--by`); seeds Todo/Doing/Done |
| `board list` | List boards (`--project` optional filter) |
| `board get <ref>` | Get by id, name, or `project/name` |
| `board update <ref>` | Update (`--name`, `--description`, `--by`) |
| `board delete <ref>` | Delete board (`--by`) |
| `board show <ref>` | Kanban view grouped by column |

### `obd column` (alias: `col`)

| Command | Description |
|---------|-------------|
| `column add` | Add column (`--board`, `--name`, `--position`, `--by`) |
| `column list` | List columns (`--board`) |
| `column rename <column>` | Rename (`--board`, `--name`, `--by`) |
| `column reorder <column>` | Reorder (`--board`, `--position`, `--by`) |
| `column delete <column>` | Delete empty column (`--board`, `--by`) |

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
| `task metadata set <id> <key> <value>` | Set a metadata key (`--by`) |
| `task metadata get <id> <key>` | Get a metadata value |
| `task metadata delete <id> <key>` | Delete a metadata key (`--by`) |
| `task metadata list <id>` | List a task's metadata key/value pairs |

**create flags:** `--board` (req), `--title` (req), `--description`, `--column`, `--assignee`, `--priority`, `--tags` (comma-separated), `--watchers` (comma-separated refs), `--by` (actor for activity)

**list flags:** `--board`, `--project`, `--assignee`, `--column` (with board), `--watcher`, `--tag`

**update flags:** `--title`, `--description`, `--assignee`, `--clear-assignee`, `--priority`, `--tags` (replace), `--by`

**move flags:** `--column` (req), `--position`, `--by`

**metadata:** keys are slugs and unique per task (setting an existing key
overwrites it); values are free-form strings. There's no bulk flag on
`create`/`update` — set keys individually with `task metadata set`.

### `obd activity`

List activity / comments globally with filters:

- `--board`, `--project`, `--task`, `--limit` (default 50)

#### Activity `data` payloads

Every mutating operation — on tasks, projects, boards, columns, and
developers — is logged. Non-comment rows carry a `data` JSON field (visible
in `--json`/`--toon` output) with structured old/new state, so previous and
next state can be reconstructed from the stream. Payloads name the affected
object in `data.entity` (`task`, `project`, `board`, `column`, `developer`).

Task payloads, by kind:

| Kind | Payload |
|------|---------|
| `created` | `{"task": <snapshot>}` — full initial state |
| `updated` | `{"changes": {"<field>": {"old": ..., "new": ...}, ...}}` |
| `moved` | `{"from": {"column", "column_id", "position"}, "to": {...}}` |
| `deleted` | `{"task": <snapshot>}` — full final state |
| `watched` / `unwatched` | `{"developer": "<username>"}` |
| `commented` | none (the `message` is the comment text) |

A task snapshot contains: `id`, `title`, `description`, `column`, `column_id`,
`priority`, `position`, `assignee` (username or null), `tags`, `watchers`,
`metadata`.

`task metadata set`/`delete` log as `updated` kind, with a synthetic field
name `metadata.<key>` in `changes` (e.g. `changes["metadata.jira-key"]`).
Setting a key to its current value is a no-op and logs nothing.

Other entities use the same scheme: `created`/`deleted` carry a full snapshot
(`data.project`, `data.board`, `data.column`, `data.developer`), `updated`
carries `data.changes`, and column reorders are `moved` with `from`/`to`
positions. Project/board/column commands accept `--by` for actor attribution,
like task commands.

Deleting something does **not** erase its history:

- Deleted **tasks**: their rows are detached (`task_id` cleared, preserved as
  `data.task_id`) and remain in the board and project streams.
- Deleted **boards**/**projects**: all their history rows are likewise
  detached (`board_id`/`project_id` move into `data`); board history stays in
  the project stream, project history remains in the global stream.
- Deleted **developers**: rows they authored keep the username in
  `data.actor` even though the `actor` join becomes empty.

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

## Releases (CI)

Pushing a version tag `v*` runs [`.github/workflows/release.yml`](.github/workflows/release.yml), which:

1. Runs `go vet` and `go test`
2. Builds **Linux amd64** (`obd-linux-amd64`) and **Windows amd64** (`obd-windows-amd64.exe`) with `CGO_ENABLED=0`
3. Embeds the tag into `obd version` via ldflags
4. Creates a GitHub Release with those binaries plus `checksums.txt`

### Create a release yourself

**From the CLI (recommended):**

```bash
# on main/master, with a clean tree, after pushes are up to date
git pull
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

Then open the **Actions** tab → workflow **Release** → when green, open **Releases** for downloads.

Equivalent with GitHub CLI (creates the annotated tag and GitHub release metadata; CI still attaches bins when the tag is on the remote):

```bash
gh release create v0.1.0 --generate-notes --title "v0.1.0"
# if the tag does not exist yet, gh creates it from the current commit
```

**From the GitHub website:**

1. Repo → **Releases** → **Draft a new release**
2. **Choose a tag** → type `v0.1.0` → **Create new tag** on the default branch
3. Title e.g. `v0.1.0` → **Publish release**  
   (Publishing creates/pushes the tag, which starts the workflow; assets appear on the release when the job finishes. If a release shell already exists without assets, re-run the workflow or re-push the tag carefully.)

If you only need the tag from the UI without drafting notes first, you can also: **Releases** → new release → create tag only and publish, or push the tag from git as above.

### Ask an agent (e.g. me) to cut a release

In chat, after the desired commits are on the remote default branch:

```text
Create release v0.1.0: tag the current origin tip, push the tag, and open/watch the GitHub release.
```

or shorter:

```text
Ship v0.1.0
```

I will then (unless you say otherwise): verify clean status, use the latest commit on the tracked remote branch, create an annotated tag `v0.1.0`, `git push origin v0.1.0`, and optionally `gh release view` / wait for Actions. **I will not force-push tags or rewrite history.**

### Local version string

```bash
go build -ldflags "-X obdurate/internal/cli.Version=v0.1.0" -o obd ./cmd/obd
./obd version   # → obd v0.1.0
```

Without ldflags, version is `dev`.

## License

[BSD 2-Clause License](LICENSE) (Simplified BSD). Copyright (c) 2026 Ivan Voras.
