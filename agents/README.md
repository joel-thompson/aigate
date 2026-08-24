# CLI Design Guidelines

A structured reference for writing well-designed command-line programs, based on [clig.dev](https://clig.dev/) — the Command Line Interface Guidelines by Aanand Prasad, Ben Firshman, Carl Tashian, and Eva Parish.

These files are intended for use by an agent working on aigate. Start with the checklist for a quick pass, then consult individual topic files for deeper guidance.

This is a filtered copy: the source guidelines' sections on interactivity, naming and distribution, analytics, and testing are omitted, because aigate doesn't prompt, doesn't collect telemetry, and keeps its own testing conventions in `CLAUDE.md`.

---

## File Index

| File                                                             | Topic                                                         |
|------------------------------------------------------------------|---------------------------------------------------------------|
| [checklist.md](./checklist.md)                                   | Quick-reference checklist — use this during implementation    |
| [01-philosophy.md](./01-philosophy.md)                           | Core design philosophy and principles                         |
| [02-basics.md](./02-basics.md)                                   | Essential requirements: exit codes, streams, argument parsing |
| [03-help-and-documentation.md](./03-help-and-documentation.md)   | Help text, examples, and documentation                        |
| [04-output.md](./04-output.md)                                   | Output formatting, JSON, verbosity                            |
| [05-errors.md](./05-errors.md)                                   | Error messages, exit codes, stderr                            |
| [06-arguments-and-flags.md](./06-arguments-and-flags.md)         | Arguments, flags, conventions, naming                         |
| [08-subcommands.md](./08-subcommands.md)                         | Subcommand structure and design                               |
| [09-robustness.md](./09-robustness.md)                           | Edge cases, idempotency, signals                              |
| [10-configuration-and-env.md](./10-configuration-and-env.md)     | Config files and environment variables                        |
| [13-extensibility.md](./13-extensibility.md)                     | Lifecycle hooks and plugin architecture                       |

File numbering is inherited from the source and has gaps where files were dropped.

---

## The Core Tension

The central challenge of CLI design is **balancing human usability with machine composability**. A good CLI:

- Defaults to human-friendly output (formatted, verbose enough to be clear)
- Degrades gracefully when piped or scripted (detects TTY, supports `--json`, `--plain`, `-q`)
- Follows conventions so users can transfer knowledge from other tools
- Communicates clearly: what happened, what went wrong, what to do next

---

## Orientation: aigate is invoked by machines

The guidelines below assume a CLI whose primary caller is a human at a terminal, with
machine-readable output as a secondary mode. aigate inverts that. It's a hook runner: Claude Code
and git invoke it as part of their lifecycles, and a human types `aigate` directly only for `init`
and `doctor`. Read every guideline through that lens.

1. **stdout is a data channel, not a display.** Whatever a hook writes to stdout is read by another
   program — Claude Code splices `claude session-start` output straight into the model's context.
   Log lines, progress chatter, and decoration on stdout are corruption, not noise. Diagnostics go
   to stderr.
2. **The exit code is a verdict.** For gate hooks, exit status *is* the product: non-zero blocks the
   action. It isn't merely a success/failure signal for a human to inspect after the fact, so treat
   every exit path as a deliberate decision.
3. **Non-interactive is the normal case.** There is usually no TTY, no one to answer a prompt, and
   no one watching a spinner. Non-interactive execution isn't a degraded fallback from an
   interactive path — it's the path. Anything that requires a human present belongs behind an
   explicit flag on `init`.
4. **Failing open is a feature.** A per-turn hook must not break the user's session because a
   provider misbehaved or `.aigate.yml` is absent. This deliberately departs from the usual "fail
   loudly on bad input" advice; see `CLAUDE.md` for where it applies.
5. **The audience for an error message may be a model.** Text aigate emits can end up in a
   conversation as an instruction. Say what happened, why, and what to do next — with no
   trailing framing that reads as a directive unless you intend it as one.

Beyond that, the ordinary questions still apply: what does the command do, what are its inputs,
what does it output, how does it fail, how is it configured, how does it explain itself.

---

*Source: [https://clig.dev](https://clig.dev) — licensed under [Creative Commons Attribution-ShareAlike 4.0](https://creativecommons.org/licenses/by-sa/4.0/)*

*This directory is a modified redistribution of that work — files have been removed, pruned, and
edited for aigate. Original authors: Aanand Prasad, Ben Firshman, Carl Tashian, and Eva Parish.
As a derivative of CC BY-SA 4.0 material, it is likewise distributed under
[CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/).*
