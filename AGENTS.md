# AGENTS.md — Obdurate development guide

Instructions for AI agents and human contributors working on this repository.

## Documentation sync rule (mandatory)

**Whenever you add, remove, rename, or change the behavior of a user-facing feature, update `README.md` in the same change.**

That includes:

- New or removed CLI commands / flags / aliases
- Domain model changes (fields, enums, relationships)
- Default DB path, output formats, exit codes
- Build / install / Makefile targets that users care about
- Workflow semantics (kanban defaults, resolution rules, activity kinds)

Preferred order when implementing work:

1. Implement code + tests (if any)
2. Update `README.md` so the command list and examples still match reality
3. Update this file if layout, architecture, or conventions changed

Do **not** leave user docs stale. If a CLI help string changes, mirror it in README.

## Product summary

- **Module:** `obdurate`
- **Binary:** `obd`
- **DB:** SQLite (`--db`, default `./db/obdurate.db`)
- **Auth:** none — all actions allowed
- **UX:** Cobra subcommands; script-friendly (`--json` / `--csv`, exit codes)

## Layout

```
cmd/obd/main.go              # entrypoint
internal/cli/                # Cobra commands + output formatting
internal/db/                 # Open SQLite, embed schema.sql, migrations (currently re-apply schema)
internal/model/              # Domain types, role/priority enums
internal/store/              # Persistence / repository layer
Makefile
README.md                    # User-facing docs — keep in sync
AGENTS.md                    # This file
```

Conventions:

- Prefer `internal/` for all non-main packages.
- Do **not** put business logic in `cli` beyond flag parsing and printing.
- Store layer owns SQL and transactions.
- Pure-Go SQLite only (`modernc.org/sqlite`); avoid CGO.

## Domain model

| Entity | Notes |
|--------|--------|
| Developer | Global; refs: `id` \| email \| username \| slack_id; roles: `admin\|lead\|developer\|viewer` |
| Project | Multiple boards |
| Board | Belongs to project; ref: `id` \| name \| `project/board` |
| Column | Per-board; default Todo, Doing, Done on create; ordered by `position` |
| Task | title, description, assignee, priority, tags, watchers, position in column |
| Activity | Unified stream: created, updated, moved, commented, watched, unwatched, deleted, … |

Priority: `low|medium|high|critical` (default medium).

## Architecture notes

### App lifecycle (`internal/cli/root.go`)

- Global flags: `--db`, `--json`, `--csv`
- `PersistentPreRunE` opens DB (creates parent dirs + applies schema) for most commands
- Skips DB for: root help, `help`, `completion`, `version`
- Exit mapping: not found → 2; invalid/conflict/exists → 3; else → 1

### Store / SQLite pitfalls

- `sql.DB` is configured with **`SetMaxOpenConns(1)`** for write serialization.
- **Never query while another `*sql.Rows` is open** on the same DB, and **never call `s.db` helpers that resolve entities from inside an open transaction that holds the sole connection** without resolving refs **before** `Begin()`.
- Pattern used in `ListTasks`: fully drain + close rows, then hydrate tags/watchers.
- Pattern used in `CreateTask`: resolve watchers/assignee/actor before `Begin()`.

### Schema

- Embedded at `internal/db/schema.sql` via `//go:embed`
- Applied on every open (`CREATE TABLE IF NOT EXISTS` / indexes)
- Foreign keys enabled; cascading deletes for project/board managers

### Output

- `internal/cli/output.go` — table (default), JSON, CSV
- `export tasks` forces JSON if neither `--json` nor `--csv` was set

## CLI command map (source of truth sites)

| Area | Package files |
|------|----------------|
| root, version, wiring | `internal/cli/root.go` |
| developer | `internal/cli/developer.go` |
| project | `internal/cli/project.go` |
| board / column | `internal/cli/board.go` |
| task | `internal/cli/task.go` |
| activity / export | `internal/cli/activity.go` |
| store | `internal/store/*.go` |

When adding a command group, register it in `NewRoot()` and document it in README.

## Build & verification

```bash
make build          # or: go build -o obd ./cmd/obd
make vet
make test           # when tests exist
./obd --help
./obd <cmd> --help
```

After meaningful CLI changes, do a short smoke path:

```bash
./obd --db /tmp/obd-smoke.db developer create --name A --email a@x --username a
./obd --db /tmp/obd-smoke.db project create --name p
./obd --db /tmp/obd-smoke.db board create --project p --name b
./obd --db /tmp/obd-smoke.db task create --board p/b --title t --by a
./obd --db /tmp/obd-smoke.db board show p/b
./obd --db /tmp/obd-smoke.db export tasks --board p/b --json
```

## Coding conventions

- Match existing style; no gratuitous comments.
- Do not commit secrets or databases (`db/`, `*.db` are gitignored).
- Prefer small, focused commits if the user asks to commit.
- IDs are int64 SQLite rowids.
- Times stored as UTC RFC3339Nano strings.

## Out of scope (unless explicitly requested)

- Authentication, multi-tenancy ACLs
- HTTP API / TUI
- Due dates / estimates (not in current model)
- Real migrations framework (schema is currently embed + IF NOT EXISTS)

If adding any of the above, document them in README and note migration strategy here.
