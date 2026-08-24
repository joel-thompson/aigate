# aigate

Claude Code lifecycle management — a configurable hook runner. Reads a per-project `.aigate.yml`

## Vocabulary

- **System** — an external tool that triggers hooks (Claude Code, git). Config splits by system
  first, so "who calls this" is unambiguous.
- **Hook** — a lifecycle event aigate intervenes on. Each has a *shape*:
  - **enrich** — produces text injected into the conversation (`claude session-start`)
  - **gate** — validates and exits non-zero to block (`claude stop`, `git pre-commit`)
- **Provider** — a configurable unit of work inside a hook, implementing `provider.Provider`.
- **Config** — per-project `.aigate.yml` at the repo root, discovered from CWD.

## Commands

```sh
go build ./...
go test ./...
go vet ./...
gofmt -l .
```

## Conventions — what the code does today

**Commands.** One package per command, each exposing `NewCommand()`, wired via explicit
`AddCommand` in the root command — no `init()` registration, no global vars. I/O only through
`cmd.OutOrStdout()`/`ErrOrStderr()`/`InOrStdin()`, never `os.Stdout`.

**Hooks fail open.** A missing `.aigate.yml` (`errors.Is(err, os.ErrNotExist)`) returns nil, not
an error — a per-turn hook can't break the session over a provider misbehaving. `RunEnrich` drops
failed providers' output rather than surfacing raw errors to Claude.

**The stop hook is special.** Blocks with exit 2 and the prompt on *stderr*, never stdout; sets
`SilenceErrors` so cobra's banner doesn't leak into the recap instruction.

**Adding a provider** touches three places: the package under `internal/provider/`, `validTypes`
in `internal/config/config.go`, and the `buildProviders`/`buildChecks` switch in the command that
uses it. Unknown types fail at `Run`, not at wiring time.

**Config edits splice raw text lines.** Never re-marshal `.aigate.yml` — it would destroy comments
and formatting. `KnownFields(true)` makes an unknown YAML key a hard error.

**Errors** wrap with context — `fmt.Errorf("reading config: %w", err)`, never bare `return err`.
Exit codes go through `internal/exit`. No `os.Exit`/`panic` outside `main.go`.

**The command layer is thin.** `internal/cmd/<command>/` parses flags and delegates to a package
under `internal/`; no business logic there. Commands import domain packages, never the reverse —
why `internal/wiring` is a leaf package rather than `doctor` importing `initcmd`.

**Exit codes:** `0` success/allow, `1` general failure (`exit.CodeFor`'s fallback; also
`doctor`'s problem-found exit), `2` stop-hook block (`stop.go:141`). Standard CLI convention
reserves 2 for usage errors; aigate overrides it because Claude Code defines 2 as "block" — a
deliberate deviation, not a bug to "fix" later.

**Exit code reports the gate verdict, not aigate's health.** Every fail-open path returns `nil`
(exit 0) with the diagnostic on stderr — an intentional inversion of "non-zero on any failure,"
not an oversight.

**Never type a return as `*exit.Error`.** Return `error`. A typed nil in an `error` interface is
non-nil, so `if err != nil` fires and every success becomes a spurious block.

**Providers are independent.** Output order follows config order and is stable, but no provider
may depend on another having run, and each must respect `ctx` — `claude stop` runs every
assistant turn, so a slow provider is a per-turn latency hit.

**Idempotency.** `init` re-runs safely against an existing `.aigate.yml`, `settings.local.json`,
and `.git/hooks/pre-commit` (`wiring.Detect` + line-splicing edits). Providers must not
accumulate state.

**Inference is disclosed, never silent.** `ciinfo.Detected`/`wiring.Detect` infer; `init`'s
`printStep` and `doctor`'s report disclose. Same for file writes and subprocess execs — the
`shell` provider execs config-supplied commands, `ci-info` shells out to `circleci`.

**Remedies are a field, not prose.** `Check.Remedy` (`wiring/report.go:289`) carries the fix
(`aigate init --claude-hooks`, etc.); `doctor` renders it as `fix: …`.

**No secrets in `.aigate.yml`.** It's committed, and the `shell` provider executes it. Secrets
come from the environment at exec time, not config.

**Group commands must reject unknown subcommands.** `RunE` that errors on unknown args, plus
`FParseErrWhitelist{UnknownFlags: true}`, on `claude` and `git`. Without it an unknown subcommand
exits 0 (looks like success) and an unknown flag after it reports as "unknown flag" instead of
"unknown command". **Unimplemented today** (`internal/cmd/claude/claude.go:10-13`,
`internal/cmd/git/git.go`) — tracked in `tickets/07-conventions-drift.md`.

## Testing

Stdlib `testing` only — no testify, assertion libraries, golden files, or mock frameworks. Keep
it that way absent a real reason.

- Tests are in-package (`package config`, not `config_test`) so unexported funcs are reachable.
- One named function per scenario, `TestFunc_Scenario`. Not table-driven.
- Hand-rolled assertions: `if got != want { t.Errorf("expected %q, got %q", want, got) }`.
- Filesystem tests use `t.TempDir()` + `t.Chdir()`. Never `os.Chdir`.
- Hand-written stubs for the `Provider` interface.
- Cobra commands are tested through their public surface: `NewCommand()`, `SetOut`, `SetArgs`,
  `Execute`, then assert on the buffer.
- Validation error strings are asserted exactly — changing a message breaks tests on purpose.
- `t.Errorf` by default so one run reports every failure; `t.Fatalf` only as a precondition gate
  where continuing would panic or be meaningless.
- Never inline a function/method call as an assertion argument — capture it in a variable first.
  Type conversions and `len` are exempt.
- Assert only on text the formatter actually emits — doubly true for the stop hook, where stderr
  is a functional output the model consumes, not a cosmetic message.
- Command-level tests assert exit code, stdout, **and** stderr separately — three independently
  load-bearing channels (gate verdict / enrich payload / block instruction). `stop_test.go:187-214`
  is the model. Seams are injected as func parameters (`exec.LookPath`, `cliRunner`) and faked by
  hand.

(Upstream also advises `t.Run` groupings for table-driven tests; aigate doesn't write those, so
that advice is dropped rather than adopted uninformed.)

## Standards for new surface area

Rules for code that doesn't exist yet — not retrofits.

- `--json` on any data-returning command, fields enumerated in help; nothing else goes to stdout
  when it's active. `doctor` is the plausible first consumer.
- Config precedence: flags > env > project `.aigate.yml` > defaults. Encapsulate in `Effective*`
  accessors; call sites never re-derive it.
- Env vars: `AIGATE_*`, mirroring the flag name, documented in help. Honor `NO_COLOR`/`CI`.
  `AIGATE_DEBUG` for tracing provider runs — hooks aren't hand-typed, so a flag is the wrong
  diagnostic channel.
- Never `os.Getenv` from a command; route env reads through the config package.
- Check for a TTY before blocking on stdin or prompting.
- Max two levels of command nesting; a third becomes a real top-level package, not a cobra alias.
- `--config` is already `init`'s boolean "create the config file" flag (`init.go:52`), colliding
  with the usual `--config <path>` convention — resolve that before adding a config-path override.
- Hook command paths are a compatibility surface, baked into `settings.local.json` and
  `.git/hooks/pre-commit` in every wired repo. Renaming one breaks installs; `doctor` is where
  stale wiring gets caught.
- Handle SIGTERM alongside SIGINT — a hook killed for being slow gets SIGTERM.
- `Long` + `Example` on human-facing commands (`init`, `doctor`, root); help under ~40 lines,
  since the primary reader is an agent that captures roughly that much. Hook verbs don't need
  worked examples.
- If binary-boundary tests are ever added, run them with `-count=1` — `go test` cache-keys on the
  test package's imports, not everything the binary compiles in, so a change downstream of the
  binary but outside the test's imports can leave a stale green result.

## Further reading

`agents/` holds deeper CLI design rationale — a filtered, aigate-scoped copy of
[clig.dev](https://clig.dev)'s CLI Design Guidelines. Deliberately excluded: interactivity,
color/animation, analytics/telemetry, naming/distribution.

## Git

Plain imperative commit subjects. No conventional-commit prefixes (`feat:`, `fix:`, `chore:`).
Write as "I", not "we".
