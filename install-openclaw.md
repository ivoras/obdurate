# Obdurate (`obd`) setup instructions for the OpenClaw agent

You are an OpenClaw agent reading this file to set up the Obdurate kanban
project-management scheme for your user. The GitHub repo for Obdurate is
at https://github.com/ivoras/obdurate .

Follow the steps in order. Ask the user before anything marked **ASK**. 
Report each step's outcome briefly.

## 1. Locate the OpenClaw root directory

The OpenClaw root is the directory that **contains your `workspace`
directory** (i.e. `workspace`'s parent). Determine it now and call it
`$OPENCLAW_ROOT` below. If you cannot determine it, **ASK** the user for it.

Create the data directory:

```bash
mkdir -p "$OPENCLAW_ROOT/obdurate"
```

The database will live at `$OPENCLAW_ROOT/obdurate/obdurate.db`. It is
created automatically on first use — no init step is needed — and comes
pre-seeded with a project named `default` (board `main`) that receives tasks
whose project the user does not specify.

## 2. Install the `obd` executable

First check whether it is already installed: `obd version` (any output means
you can skip to step 3).

Preferred: download the latest release binary from GitHub:

- Linux amd64: asset `obd-linux-amd64` from
  `https://github.com/ivoras/obdurate/releases/latest`
- Windows amd64: asset `obd-windows-amd64.exe`

Example (Linux):

```bash
mkdir -p "$OPENCLAW_ROOT/obdurate/bin"
curl -fL -o "$OPENCLAW_ROOT/obdurate/bin/obd" \
  https://github.com/ivoras/obdurate/releases/latest/download/obd-linux-amd64
chmod +x "$OPENCLAW_ROOT/obdurate/bin/obd"
```

Verify with `"$OPENCLAW_ROOT/obdurate/bin/obd" version`. Optionally also
verify the download against `checksums.txt` from the same release.

If no release asset fits the platform (e.g. macOS or arm64), build from
source instead — requires a current Go toolchain:

```bash
git clone https://github.com/ivoras/obdurate /tmp/obdurate-src
cd /tmp/obdurate-src && go build -o "$OPENCLAW_ROOT/obdurate/bin/obd" ./cmd/obd
```

**ASK** the user whether they also want `obd` on their PATH (e.g. symlinked
into `~/.local/bin`); otherwise always invoke it by its full path.

## 3. Wire up the database path

`obd` takes the database location via `--db`. Every invocation you make must
use:

```bash
"$OPENCLAW_ROOT/obdurate/bin/obd" --db "$OPENCLAW_ROOT/obdurate/obdurate.db" ...
```

Do a smoke test now (uses a throwaway database, then removes it):

```bash
"$OPENCLAW_ROOT/obdurate/bin/obd" --db /tmp/obd-smoke.db project create --name smoketest
"$OPENCLAW_ROOT/obdurate/bin/obd" --db /tmp/obd-smoke.db project list --json
rm -f /tmp/obd-smoke.db
```

Both commands must exit 0 and the second must print a JSON array.

## 4. Install the agent skill (optional)

**ASK** the user: "Do you want me to download the obd project-management
skill from GitHub and install it into the OpenClaw skills directory? It
teaches me to handle requests like 'add a task', 'move it to done', or
'show the board'."

If yes:

```bash
SKILL_DIR="$OPENCLAW_ROOT/skills/obd-project-management"
mkdir -p "$SKILL_DIR"
curl -fL -o "$SKILL_DIR/SKILL.md" \
  https://raw.githubusercontent.com/ivoras/obdurate/master/skills/obd-project-management/SKILL.md
curl -fL -o "$SKILL_DIR/reference.md" \
  https://raw.githubusercontent.com/ivoras/obdurate/master/skills/obd-project-management/reference.md
echo "$OPENCLAW_ROOT/obdurate/obdurate.db" > "$SKILL_DIR/DB_PATH"
```

The `DB_PATH` file is read by the skill to find the database — it must
contain the absolute path from step 1, one line, no quotes. If your OpenClaw
installation keeps skills somewhere other than `$OPENCLAW_ROOT/skills`,
install into that directory instead and keep all three files together.

## 5. Confirm to the user

Tell the user:

- where the `obd` binary and database are,
- whether the skill was installed and where,
- that they can now ask things like "create a project called X", "add a task
  to the board", or "show me recent activity" in normal conversation.

Suggested first steps for the user: create a developer record for themselves
(`developer create --name ... --email ... --username ...`), then a project
and a board.
