package ciinfo

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

const cliBinary = "circleci"

const cliTimeout = 3 * time.Second

// cliHelpLine points Claude at the CLI's own help instead of reproducing it
// here. The binary's help is always current, and enumerating its commands
// cost ~40 lines of session-start context to answer up front what one
// `circleci help` answers on demand.
const cliHelpLine = "Run `circleci help` to list commands, or `circleci <cmd> --help` for one command's flags."

// cliRunner shells out to the circleci CLI. Injectable so tests never depend
// on whether the machine running them has the CLI installed.
type cliRunner func(ctx context.Context, args ...string) (string, error)

// execCLI is the real cliRunner. It captures stdout only (matching
// shell.Provider), so any ANSI escapes or telemetry prompts the CLI writes
// to stderr never reach Claude's context. NO_COLOR keeps the captured text
// plain; CIRCLE_NO_TELEMETRY keeps this background probe from polluting the
// user's own CLI telemetry with commands they didn't run.
func execCLI(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, cliBinary, args...)
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "CIRCLE_NO_TELEMETRY=1")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// cliSection reports that the local circleci CLI is available and which
// version it is, discovered from the binary rather than hardcoded. It
// returns "" on any failure and never an error: RunEnrich prints "[error]
// %v" to stderr for a returned error, and erroring here would produce that
// noise at every session start on any machine without the CLI.
func cliSection(ctx context.Context, runCLI cliRunner) string {
	ctx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()

	version, err := runCLI(ctx, "version")
	if err != nil {
		return ""
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	return "CLI · " + version + "\n" + cliHelpLine
}
