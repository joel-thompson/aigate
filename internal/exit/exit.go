// Package exit carries specific process exit codes up through error returns.
package exit

import (
	"errors"
	"fmt"
)

// Error carries a specific process exit code up to main.
type Error struct{ Code int }

func (e *Error) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// CodeFor returns the exit code an error should produce: 1 unless the error
// carries an explicit code.
func CodeFor(err error) int {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Code
	}
	return 1
}
