package skillscli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeScript installs an executable shell script that stands in for the
// skills binary: it echoes what it saw so the test can assert what the
// child process actually received.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-skills")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExecRunsTheChildWithArgsEnvAndClosedStdin(t *testing.T) {
	script := writeScript(t, `echo "args: $@"
echo "env: $FLEET_TEST_ENV"
if read -r line; then echo "prompted: $line"; else echo "stdin: EOF"; fi
`)
	inv := Invocation{
		Exe:   script,
		Args:  []string{"update", "-g", "-y"},
		Env:   append(os.Environ(), "FLEET_TEST_ENV=on"),
		Stdin: mustClosedStdin(t),
	}

	res, err := Exec{}.Run(inv)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, want := range []string{"args: update -g -y", "env: on", "stdin: EOF"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("child output missing %q:\n%s", want, res.Stdout)
		}
	}
}

func TestExecReportsAFailedRun(t *testing.T) {
	script := writeScript(t, `echo "upgrading tdd"
echo "boom: disk full" >&2
exit 3
`)
	inv := Invocation{Exe: script, Args: []string{"update"}, Env: os.Environ(), Stdin: mustClosedStdin(t)}

	res, err := Exec{}.Run(inv)
	if err == nil {
		t.Fatal("Run() succeeded on a failing child, want error")
	}
	if !strings.Contains(res.Stdout, "upgrading tdd") || !strings.Contains(res.Stderr, "boom: disk full") {
		t.Errorf("output not captured: %+v", res)
	}
	if !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("error = %v, want the exit status", err)
	}
}

// mustClosedStdin exposes the wrapper's closed pipe for runner-level tests.
func mustClosedStdin(t *testing.T) *os.File {
	t.Helper()
	f, err := closedStdin()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() }) //nolint:errcheck // test cleanup
	return f
}
