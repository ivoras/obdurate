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
- **Go version:** track current stable (see `go` line in `go.mod`; do not pin an outdated LTS like 1.22 unless required by a dep floor)
- **DB:** SQLite (`--db`, default `./db/obdurate.db`)
- **Auth:** none — all actions allowed
- **UX:** Cobra subcommands; script-friendly (`--json` / `--csv` / `--toon`, exit codes)

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
| Project | Multiple boards. Names are slugs (`normalizeSlug` in store.go: lowercase ascii/digits/`-`/`_`, ≤64 chars; input auto-lowercased). A fresh DB is seeded with project `default` + board `main` (`store.EnsureDefaults`, called from root.go; only when zero projects exist) |
| Board | Belongs to project; ref: `id` \| name \| `project/board`. Names are slugs (same rules as projects) |
| Column | Per-board; default Todo, Doing, Done on create; ordered by `position` |
| Task | title, description, assignee, priority, tags, watchers, position in column |
| Activity | Unified stream covering ALL mutations (tasks, projects, boards, columns, developers): created, updated, moved, commented, watched, unwatched, deleted. Each row has a `data` JSON payload (`data.entity` + old/new values or snapshots) so state transitions are reconstructible; payload shapes are documented at the top of `internal/store/activity.go` and in README. Deletions detach (never cascade-delete) history: ids move into `data.task_id`/`data.board_id`/`data.project_id`, and deleted developers' authorship is kept in `data.actor`. When adding a store mutation, log it in the same tx via `addActivityTx` and accept an actor ref (`--by`). |

Priority: `low|medium|high|critical` (default medium).

## Architecture notes

### App lifecycle (`internal/cli/root.go`)

- Global flags: `--db`, `--json`, `--csv`, `--toon`
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
- Column additions to existing tables need an explicit `ensureColumn` call in
  `internal/db/db.go` (`CREATE TABLE IF NOT EXISTS` never alters old tables)

### Output

- `internal/cli/output.go` — table (default), JSON, CSV, TOON (`github.com/toon-format/toon-go`)
- Mutually exclusive: `--json`, `--csv`, `--toon`
- Structured object dumps use `PreferStructured()` + `PrintStructured()` (JSON or TOON)
- `export tasks` forces JSON if no machine-readable format flag was set

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

## Releases / CI

- Workflow: `.github/workflows/release.yml`
- Trigger: push of tags matching `v*`
- Builds: `obd-linux-amd64`, `obd-windows-amd64.exe`, `checksums.txt` → GitHub Release
- Version: `-ldflags "-X obdurate/internal/cli.Version=<tag>"` (default `dev`)

When the user asks to ship/release:

1. Ensure working tree is clean and default branch matches remote tip
2. Create annotated tag `vX.Y.Z` and push `origin vX.Y.Z`
3. Do not force-push tags
4. Prefer `gh` to show release/Actions status if available
5. **After CI creates the release, write its description from the commit
   messages that make up the release** (see below)

### Release descriptions (mandatory)

Every release gets a description generated from the commits between the
previous tag and this one — do this every time a release is created:

1. Collect them: `git log --reverse previous-tag..vX.Y.Z` (full messages,
   not just subjects; for the first release, from the repo root).
2. Write a short prose summary grouped by theme (features / fixes / docs),
   derived only from those commit messages — do not invent items that are
   not in the commits. Subject lines map to bullets; commit bodies supply
   the detail worth surfacing.
3. Apply it: `gh release edit vX.Y.Z --notes-file <file>` once the CI-created
   release exists (CI's auto-generated notes are only a placeholder). Keep
   the auto-generated "Full Changelog" compare link at the bottom.

### Release code names (mandatory)

Every release gets a unique, whimsical **code name derived from the works of
Stephen King** — not just novel/novella/story titles: characters (*Annie
Wilkes*, *Randall Flagg*), famous situations or places (*Room 217*, *the
Overlook*, *Shawshank*), and trope names (*the Deadlights*, *Ka*, *the
Thinny*) are all fair game — chosen to relate to the release description.
E.g. a first release might be *Carrie* (King's first novel), a release about
preserving deleted history *Pet Sematary*, one that escapes a long-standing
bug *Shawshank*.

1. Pick a title, character, situation, or trope whose theme echoes the
   release's content; a one-line justification in the release notes is
   welcome but optional.
2. Uniqueness: never reuse a name — check `gh release list` first.
3. Set the release title to `vX.Y.Z - <code name>`:
   `gh release edit vX.Y.Z --title "vX.Y.Z - <code name>"`
   (do this together with applying the release description).

Keep README **Releases (CI)** section in sync if the workflow or assets change.

## Build & verification

```bash
make build          # or: go build -o obd ./cmd/obd
make vet
make test           # store/db/cli test suites; keep green and extend with behavior changes
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
- Times stored as fixed-width UTC RFC3339 strings, second granularity
  (`2006-01-02T15:04:05Z`) — lexicographic order == chronological order.
  `parseTime` still reads legacy RFC3339Nano values from older databases.

## Out of scope (unless explicitly requested)

- Authentication, multi-tenancy ACLs
- HTTP API / TUI
- Due dates / estimates (not in current model)
- Real migrations framework (schema is currently embed + IF NOT EXISTS)

If adding any of the above, document them in README and note migration strategy here.
