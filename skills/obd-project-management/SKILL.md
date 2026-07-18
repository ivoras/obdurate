---
name: obd-project-management
description: >
  Manage projects, kanban boards, and tasks with the `obd` CLI (Obdurate).
  Use this skill whenever the user asks to track work or organize a project,
  including colloquial requests such as: "add a task/ticket/card/issue",
  "create a todo", "move X to done", "mark X as in progress", "assign this to
  Alice", "who is working on what", "what's on my plate", "show the board",
  "add a comment on task 5", "start watching", "set up a new project/sprint",
  "add a team member", "what happened on the project", "show recent activity",
  "rename the column", "tag this as urgent", "delete that task", or "export
  the tasks". Also use it when the user mentions kanban, backlog, sprint
  boards, lists/lanes/stages on a board, task priorities, or task assignment
  and a local obd database exists.
---

# Managing projects with the obd CLI

`obd` is a local CLI kanban tool. All state lives in one SQLite file. There is
no authentication and no server: you perform every action by running `obd`
commands with the Bash tool and reading their output.

## Data model — how everything nests

```
Project ──> Board(s) ──> Column(s) ──> Task(s)
Developer (global; assigned to / watching tasks anywhere)
Activity  (global event stream; rows point at task/board/project)
```

- A **project** contains any number of **boards** (a board often represents a
  sprint or a workstream).
- A **board** contains ordered **columns** — what other tools call *lists*,
  *lanes*, or *stages*. New boards start with Todo / Doing / Done. A column
  belongs to exactly one board; two boards never share columns.
- A **task** (= ticket / issue / card) lives on exactly one board, in exactly
  one of that board's columns, at a position within the column.
- Tasks **CAN move freely between columns/lists of their own board** — that
  is the normal kanban flow (`task move <id> --column Doing`). Tasks can
  **NEVER move to a different board** — to "move" one across boards,
  recreate it on the target board and delete the original.
- **Developers** are global, not members of any project: anyone can be
  assigned to or watch any task in any project.
- **Tags** are global labels shared across all tasks.
- **Activity** is one global stream; entries reference the task, board, and
  project they concern, so it can be filtered at any level.

So: to create a task you need a board (hence `--board project/board`); to
create a board you need a project; columns only make sense within one board.

## Step 0 — locate the binary and the database (do this once per session)

1. **Binary**: try `obd version`. If not on PATH, look for `./obd` in the
   project root, or build it with `go build -o obd ./cmd/obd` if the obdurate
   source tree is present.
2. **Database path** — resolve in this order and remember your choice:
   1. If the environment variable `OBD_DB` is set, use its value.
   2. If a file named `DB_PATH` exists in the same directory as this skill
      file, read the path from it (one line, may contain `~`; expand it).
   3. Otherwise use the default `./db/obdurate.db` (relative to the current
      working directory).
3. **Always pass the database explicitly** on every command:
   `obd --db <path> ...`. Never omit `--db` unless you intend the default.
   The database file and parent directory are created automatically on first
   use; no init command exists.

## The Default project

Every fresh database is seeded with a project named `default` containing a
board named `main` (columns Todo / Doing / Done).

- **When the user does not say which project or board a new task/issue
  belongs to, create it on `default/main`** — do not ask, just mention it in
  your confirmation ("...added to the default project").
- If `default/main` turns out not to exist (someone deleted it), recreate
  what is missing: `project create --name default`, then
  `board create --project default --name main`.
- If the user names a project but no board, and that project has exactly one
  board, use it; if it has several, ask which one.

## Project and board names are slugs

Project and board names must be lowercase ASCII slugs: letters, digits, `-`
or `_`, starting and ending with a letter or digit, max 64 characters. `obd`
lowercases input itself but REJECTS names with spaces or other characters
(exit 3).

- When the user gives a human title ("create a project called My Cool App"),
  derive the slug yourself (`my-cool-app`: lowercase, spaces → `-`, drop
  other characters) and keep the original title in `--description`:
  `project create --name my-cool-app --description "My Cool App"`.
  Tell the user the slug you chose.
- References are case-insensitive, so `Widget` finds the project `widget`.
- Column names and task titles are NOT slugs — they are free-form text.

## Rules that prevent mistakes

- **Always add `--json`** when you need to read results (parse IDs, list
  tasks, check state). Only omit it when showing pretty output directly to
  the user. JSON lists are always arrays; an empty result is `[]`.
- **Check exit codes**: `0` success, `2` not found, `3` invalid input /
  duplicate / conflict, `1` other error. Error text is on stderr, prefixed
  `error:`. On exit 2, tell the user the item doesn't exist and offer to list
  what does exist — do not silently create it. On exit 3, read the message
  and fix the input.
- **Refer to boards as `project/board`** (e.g. `--board widget/sprint-1`).
  Bare board names fail with a conflict error when two projects have a board
  with the same name.
- **Developers** can be referenced by numeric id, email, username, or Slack
  id, case-insensitively. Prefer usernames. If a name like "Alice" is given,
  run `obd --db <path> developer list --json` and match against `name`,
  `username`, and `email`; ask the user if more than one candidate matches.
- **Pass `--by <actor>`** on every mutating command — task, project, board,
  and column create/update/move/delete/comment all accept it — whenever you
  know who is acting (the user's identity or whoever they say is acting).
  This attributes the activity-log entry. If you don't know, omit it.
- **`--tags` REPLACES the whole tag list.** To add one tag: first `task get
  <id> --json`, take the existing `tags` array, append the new tag, and pass
  the full comma-separated list.
- **Columns are per-board** and matched by name case-insensitively. New
  boards start with `Todo`, `Doing`, `Done`. Before moving a task to a column
  you have not seen, verify it exists with
  `obd --db <path> column list --board <project/board> --json`.
- **Do not edit the SQLite file directly.** Always go through `obd`.
- **Deletes are permanent** (a task's history is preserved in the activity
  stream, but the task itself is not recoverable). Confirm with the user
  before `task delete`, `board delete`, `project delete`, or
  `developer delete` unless they explicitly asked for the deletion.

## Mapping colloquial requests to commands

| The user says | You run |
|---|---|
| "add a task/ticket/card/issue/todo X" | `task create --board <project/board> --title "X"` (no project mentioned → board `default/main`) |
| "mark X done", "move X to done", "finish X" | `task move <id> --column Done` |
| "start X", "X is in progress" | `task move <id> --column Doing` |
| "assign X to NAME" | `task update <id> --assignee <ref>` |
| "unassign X" | `task update <id> --clear-assignee` |
| "make X urgent / high priority" | `task update <id> --priority high` (or `critical`) |
| "tag X as Y" | read current tags, then `task update <id> --tags "<old...,Y>"` |
| "comment on X: ..." | `task comment <id> --message "..." --by <actor>` |
| "what's on my plate", "my tasks", "NAME's tasks" | `developer tasks <ref> --json` |
| "everything in project X", "all X tasks" | `project tasks <ref> --json` |
| "show the board", "how is the sprint going" | `board show <project/board>` |
| "who's working on what" | `task list --board <project/board> --json`, group by `assignee` |
| "what happened recently / on project X" | `activity --project <ref> --json` |
| "why/when did task X change" | `task activity <id> --json`, read `data` payloads |
| "new project X" | `project create --name X` |
| "new sprint/board X" | `board create --project <ref> --name X` |
| "add NAME to the team" | `developer create --name ... --email ... --username ...` (ask for missing fields) |
| "watch/follow task X" | `task watch <id> --by <ref>` |
| "add a Review column" | `column add --board <project/board> --name Review --position <n>` |
| "export the tasks" | `export tasks --board <ref> --json` (or `--csv`) |

When the user names a task by title instead of id, find the id first:
`task list --json` (optionally filtered by board/project) and match the
`title` field case-insensitively. If several match, show them and ask.

## Day-to-day software development operations

Treat "issue", "bug", "ticket", "story", and "task" as the same thing: a
task. Model developer-workflow requests like this:

- **"Create an issue for project X and user Y"** (or "...assign it to Y"):
  1. Resolve project X (`project get X --json`; if missing, ask before
     creating it). Pick its board (sole board, or ask; no project given →
     `default/main`).
  2. Resolve user Y against `developer list --json`. If Y has no developer
     record, ask for name/email/username and create one.
  3. `task create --board <project/board> --title "..." --assignee <Y>
     --by <actor>` — include `--description`, `--priority`, `--tags` when
     the user gave that information.
- **"File a bug: ..."** — a task tagged `bug`: include `--tags bug` (plus
  any other tags) and put reproduction details in `--description`. Severity
  words map to priority: "blocker/critical" → `critical`, "major/serious" →
  `high`, "minor/cosmetic" → `low`, otherwise `medium`.
- **"What bugs are open?"** — `task list --tag bug --json` (add `--project`
  or `--board` if given) and report tasks whose `column_name` is not `Done`.
- **"Close issue N" / "resolve" / "fix confirmed"** — `task move N --column
  Done --by <actor>`. **"Reopen issue N"** — `task move N --column Todo`.
  There is no separate closed state: Done is the terminal column.
- **"I'm picking up N" / "working on N"** — `task update N --assignee <user>
  --by <user>` then `task move N --column Doing --by <user>`.
- **"Send N to review" / "ready for QA"** — move it to the board's `Review`
  (or `QA`) column if one exists (`column list`); if not, ask whether to add
  one (`column add --board <ref> --name Review --position 2`) or use Done.
- **"Plan the sprint" / "set up sprint 5"** — a new board in the project:
  `board create --project <ref> --name sprint-5`; move carried-over tasks is
  NOT possible across boards — tasks belong to one board; recreate them on
  the new board if the user wants that, and say you did so.
- **Standup / status summaries** ("what did we do yesterday", "project
  status") — combine `board show <ref> --json` for current state with
  `activity --project <ref> --json` for recent changes, and summarize per
  developer or per column.
- **Estimates and due dates are not supported.** If asked, say so and offer
  the closest substitutes: priority, tags (e.g. `--tags "due-2026-08-01"`),
  or a note in the description.

## Recipe: bootstrap a fresh setup

If a command fails with "not found" because nothing exists yet, build the
hierarchy in this order (each step needs the previous one):

```bash
obd --db <path> developer create --name "Alice Smith" --email alice@example.com --username alice
obd --db <path> project create --name myproject --description "..."
obd --db <path> board create --project myproject --name main
obd --db <path> task create --board myproject/main --title "First task" --by alice
```

## Recipe: task history and state reconstruction

Every activity entry has a structured `data` JSON field (in `--json` output)
describing the change, so you can answer "what did this look like before?":

- `created` / `deleted`: `data.task` is a full snapshot (title, description,
  column, priority, position, assignee, tags, watchers).
- `updated`: `data.changes.<field>.old` and `.new` for each changed field.
- `moved`: `data.from` / `data.to` with `column` and `position`.
- Comments carry the text in `message`; history of a deleted task remains in
  the board/project stream with the original id in `data.task_id`.

To reconstruct a task's state at a point in time: start from the `created`
snapshot, then apply each later `updated`/`moved` entry's new values in
chronological order (entries are returned newest-first — reverse them).

## Reporting back to the user

- After a mutation, confirm with the concrete result ("Created task #7
  'Fix login' in Todo on widget/sprint-1"), taking values from the command's
  JSON output — not from your assumptions.
- For listings, prefer a short readable summary or table over raw JSON.
- Never invent ids, columns, or usernames: every id you mention must come
  from command output.

For the complete flag-by-flag command reference, read `reference.md` in this
skill directory.
