# aigate

A configurable hook runner for Claude Code and git. What it does in a given repo is driven
entirely by a per-project `.aigate.yml`: session context to inject, a recap prompt to feed back,
checks to run before a commit.

## Install

Building from source needs a Go toolchain.

```sh
git clone https://github.com/joelthompson/aigate
cd aigate
./install.sh
```

`install.sh` builds the binary and copies it to `/usr/local/bin`, using `sudo` if that directory
isn't writable. Override the destination with `INSTALL_DIR`:

```sh
INSTALL_DIR=~/bin ./install.sh
```

Wherever it lands, **`aigate` must resolve on `$PATH` by bare name** — the hooks aigate registers
invoke it unqualified (`aigate claude session-start`), so an absolute-path-only install produces
hooks that silently never fire.

Then set up each project:

```sh
cd /path/to/your/project
aigate init          # interactive; prompts per feature
aigate doctor        # verify the wiring
```

`init` is append-only and idempotent — it never rewrites what's already there, so re-running it is
safe. To skip the prompts, name the features as flags:

```sh
aigate init --config --claude-hooks --git-hooks
```

Passing any flag switches `init` to non-interactive mode; unnamed features are skipped.

## Hooks

| System | Hook | Shape | Command |
|---|---|---|---|
| Claude Code | SessionStart | enrich | `aigate claude session-start` |
| Claude Code | Stop | gate | `aigate claude stop` |
| git | pre-commit | gate | `aigate git pre-commit` |

### session-start (enrich)

Runs the providers under `claude.session-start.context` in config order and joins their output
with blank lines, each section under a `## <Label>` heading, on **stdout** — which Claude Code
injects into the session. Always exits 0. A provider that fails is dropped from the output and its
error goes to stderr, so a broken provider never leaks a raw error into the model's context.

### stop (gate)

Reads Claude Code's hook JSON on **stdin**. When `claude.stop-prompt` is enabled it exits **2**
with the prompt on **stderr**, which Claude Code reads as the instruction to continue with. Stdout
stays empty. It allows the stop (exit 0) when:

- `stop_hook_active` is set in the payload — the loop guard, checked before anything else
- the `claude.stop-prompt` section is absent, or `enabled: false`
- `min-length` is set and the reply is already shorter than that many characters

### pre-commit (gate)

Runs the checks under `git.pre-commit.checks` against `git diff --cached`. Findings go to
**stderr** and a failing check exits 1, which aborts the commit.

### Wiring

`aigate init` registers the Claude Code hooks in `.claude/settings.local.json` — the per-developer
settings file, so hook registration stays out of git — and git's hook in `.git/hooks/pre-commit`.
Only `.aigate.yml` is meant to be committed.

Every hook fails open on a missing `.aigate.yml`: nothing configured means nothing to do, not an
error.

## Configuration

`.aigate.yml` lives at the repo root and is discovered from the current directory. Every key:

```yaml
claude:
  session-start:
    context:
      - type: project-structure
        max_depth: 3          # optional, default 3
      - type: shell
        command: "go-task --list"   # required for shell
        label: "Available tasks"    # optional; omitted means no heading
      - type: ci-info
      - type: docker-compose
        files:                # optional; empty means let docker discover
          - docker-compose-services.yml
      - type: git-state
  stop-prompt:
    enabled: true
    prompt: "type a clean recap..."   # optional; empty uses the built-in default
    min-length: 400                   # optional; 0 or absent means always ask

git:
  pre-commit:
    checks:
      - type: secrets-scan
```

`type` is required on every entry. `max_depth` is the one snake_case key; everything else is
kebab-case.

### Providers

**`project-structure`** — session-start. Prints a `tree`-style listing of the repo, honoring
`.gitignore` and always skipping `.git`, `node_modules`, `vendor`, `__pycache__` and `.venv`.
Caps at 50 entries per directory, disclosing how many it dropped. Scaffolded by
`aigate init --claude-hooks`.

- `max_depth` (int, default 3) — how many directory levels to descend.

**`shell`** — session-start. Runs an arbitrary command through `sh -c` and injects its stdout. A
non-zero exit is reported as a provider error and the section is dropped.

- `command` (string, **required**) — the command to run.
- `label` (string, optional) — the `## <Label>` heading. Without it the output is injected with no
  heading.

Not offered by `init`; `init` scaffolds it commented out as an example. `.aigate.yml` is committed
and this provider executes it, so keep secrets out of it — they belong in the environment at exec
time.

**`ci-info`** — session-start, no config. Summarizes the repo's CircleCI setup: the entry config's
path and version, whether it's static or a dynamic setup config, workflow and job counts, other
configs in `.circleci/`, and the `circleci` CLI version when the CLI is installed. Labeled `CI`.
Auto-offered by `init` when `.circleci/` holds at least one CircleCI-looking config.

**`docker-compose`** — session-start. Summarizes the project's compose stack: services, images,
ports, and which ones are already running, so the agent doesn't restart a live stack. Labeled
`Docker stack`. Caps at 30 services. Auto-offered by `init` when a compose file is present in the
repo root.

- `files` (list of strings, optional) — compose files to use, for projects whose file docker's own
  discovery won't find. Empty lets docker discover.

**`git-state`** — session-start, no config. Reports the branch and its upstream, ahead/behind
counts, how dirty the tree is, whether an operation like a rebase is half-finished, recent commit
subjects, and — in a linked worktree — which of the main checkout's gitignored files this worktree
is missing. Labeled `Git (snapshot at session start, not live)`, since the block stays in context
for the whole session while the state it reports moves on. Auto-offered by `init` when the
directory is a git checkout.

It reports gitignored *paths* and never reads one: naming `.env` is the value, printing it would be
the risk.

**`secrets-scan`** — pre-commit, no config. Scans added lines in the staged diff for AWS access
keys, private key blocks, GitHub personal access tokens, `sk-`-prefixed API keys, and Slack bot and
user tokens. Reports `file:line: rule` per finding. Scaffolded by `aigate init --git-hooks`.

### Validation

`.aigate.yml` is parsed with unknown fields rejected, so a misspelled key is a hard error rather
than a silently ignored line:

```
Error: loading config: parsing config: yaml: unmarshal errors:
  line 3: field contxt not found in type config.SessionStartConfig
```

An unrecognized `type` is likewise fatal, and the message points at the exact entry:

```
Error: loading config: validating config: claude.session-start.context[0]: unknown provider type "nonexistent-provider"
```

Validation is hook-agnostic, though — every provider type is valid everywhere. Putting a
pre-commit check in the session-start list passes validation and fails when the hook runs
(`[error] unsupported provider type "secrets-scan"`).

## Example output

`aigate claude session-start` on stdout, with `project-structure` and `git-state` configured:

```
## Project structure
.
├── cmd
│   └── aigate
│       └── main.go
├── internal
│   ├── config
│   ├── provider
│   └── runner
├── .aigate.yml
├── go.mod
└── install.sh

## Git (snapshot at session start, not live)
main → origin/main · ahead 2 · 1 staged, 1 modified
Main checkout /Users/you/src/project
Recent: 2c872c7 Ignore the built aigate binary
        f328764 Add git-state session-start context provider
        f036276 Add docker-compose session-start context provider
```

`aigate git pre-commit` on stderr when a check blocks the commit (exit 1):

```
[secrets-scan] FAILED
Potential secrets detected:
  path/to/file.go:3: AWS access key
Error: pre-commit checks failed
```

`aigate doctor` — one line per check, problems expanded with the path and the fix:

```
aigate doctor
cwd: /Users/you/src/project

Binary: ok (aigate resolves to /usr/local/bin/aigate)
Config: ok (.aigate.yml)
Claude settings: ok (.claude/settings.local.json)
Session context: ok
CI context: not applicable (no .circleci directory)
Stop prompt: PROBLEM
  Stop hook in .claude/settings.local.json is present, but claude.stop-prompt section in .aigate.yml is missing; the hook fires but has nothing to run
  path: /Users/you/src/project/.aigate.yml
  fix: aigate init --stop-prompt
Pre-commit checks: not configured (no aigate line in .git/hooks/pre-commit, no git.pre-commit section in .aigate.yml)
Docker context: not applicable (no compose file in the project root)
Git state: ok

1 problem found.
```

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success — the gate allows, or the command ran clean |
| `1` | General failure; also `doctor` finding at least one problem |
| `2` | The stop hook blocking, with the prompt on stderr |

`2` deviates from the usual CLI convention of reserving it for usage errors. That's deliberate:
Claude Code defines exit 2 as "block", so the stop hook has to use it.
