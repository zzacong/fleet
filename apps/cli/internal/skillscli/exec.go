// The real runner: os/exec. The child's stdin is whatever the invocation
// carries — the wrapper's closed pipe — so a prompt reads EOF instead of
// hanging. Both output streams are captured for failure display; nothing
// reaches fleet's own stdout or stderr.
package skillscli

import (
	"bytes"
	"os/exec"
)

// Exec runs invocations with os/exec: the production Runner.
type Exec struct{}

// Run implements Runner. A non-zero exit or a start failure returns as
// the error; the captured streams come back regardless, so the caller can
// show what the CLI said.
func (Exec) Run(inv Invocation) (Result, error) {
	cmd := exec.Command(inv.Exe, inv.Args...) //nolint:gosec // inv.Exe is fleet's fixed "skills" or a test script
	cmd.Env = inv.Env
	cmd.Stdin = inv.Stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return Result{Stdout: stdout.String(), Stderr: stderr.String()}, err
}
