// Package skillscli wraps the skills CLI — fleet's install and update
// backend — as a thin, non-interactive process call. Every invocation
// carries fully explicit flags and stdin piped closed, so an unexpected
// prompt fails fast instead of hanging the caller. The wrapper captures
// output for failure display and never parses the CLI's prose: fleet
// reports from its own state. The CLI's lockfile is read-only here, and
// running the skills CLI by hand keeps working.
package skillscli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Invocation is one command execution request: everything a Runner needs.
type Invocation struct {
	// Exe is the executable name or path, resolved from PATH by the
	// runner when it is not a path.
	Exe string
	// Args are the arguments, flags included, exactly as they go to the
	// child process.
	Args []string
	// Env is the child's environment.
	Env []string
	// Stdin is what the child reads. The wrapper always hands a pipe
	// whose write end is already closed, so any read yields EOF.
	Stdin io.Reader
}

// Result is what a completed invocation produced.
type Result struct {
	Stdout string
	Stderr string
}

// Runner executes one invocation. The seam exists because the skills
// binary is absent in test sandboxes; the real runner is Exec.
type Runner interface {
	Run(Invocation) (Result, error)
}

// RunnerFunc adapts a function to Runner.
type RunnerFunc func(Invocation) (Result, error)

// Run implements Runner.
func (f RunnerFunc) Run(inv Invocation) (Result, error) { return f(inv) }

// Update runs `skills update -g -y`: the skills CLI's global, non-
// interactive update, with every flag explicit. A failure comes back as
// an *Error carrying the CLI's captured output for display.
func Update(runner Runner) (Result, error) {
	return run(runner, []string{"update", "-g", "-y"})
}

// run builds the invocation for one skills CLI command and hands it to
// the runner. Stdin is a pipe closed before the child starts, so a prompt
// the explicit flags were meant to avoid reads EOF and fails fast.
func run(runner Runner, args []string) (Result, error) {
	stdin, err := closedStdin()
	if err != nil {
		return Result{}, fmt.Errorf("open stdin pipe: %w", err)
	}
	defer stdin.Close() //nolint:errcheck // read end: closing twice is harmless

	res, runErr := runner.Run(Invocation{
		Exe:   "skills",
		Args:  args,
		Env:   os.Environ(),
		Stdin: stdin,
	})
	if runErr != nil {
		return res, &Error{
			Args:   args,
			Output: strings.TrimSpace(strings.TrimSpace(res.Stdout) + "\n" + strings.TrimSpace(res.Stderr)),
			Err:    runErr,
		}
	}
	return res, nil
}

// closedStdin returns the read end of a pipe whose write end is already
// closed: the child's stdin reads EOF immediately.
func closedStdin() (*os.File, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		r.Close() //nolint:errcheck // best effort on an already-failing path
		return nil, err
	}
	return r, nil
}

// Error is a failed skills CLI run. Output is the CLI's captured output —
// the caller shows it raw so the failure stays diagnosable; the wrapper
// never interprets it.
type Error struct {
	// Args is the invocation that failed, for the error message.
	Args []string
	// Output is the CLI's combined output, trimmed.
	Output string
	// Err is the underlying process failure (non-zero exit, start
	// failure, ...).
	Err error
}

// Error implements error.
func (e *Error) Error() string {
	return fmt.Sprintf("skills %s: %v", strings.Join(e.Args, " "), e.Err)
}

// Unwrap exposes the underlying process failure.
func (e *Error) Unwrap() error { return e.Err }
