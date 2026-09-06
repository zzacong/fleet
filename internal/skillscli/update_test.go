package skillscli

import (
	"errors"
	"io"
	"reflect"
	"testing"
)

// recordingRunner captures the invocation Update builds and returns a
// scripted result — the stub that stands in for the absent skills binary.
type recordingRunner struct {
	inv Invocation
	res Result
	err error
}

func (r *recordingRunner) Run(inv Invocation) (Result, error) {
	r.inv = inv
	return r.res, r.err
}

func TestUpdateRunsTheSkillsCLIWithExplicitFlags(t *testing.T) {
	r := &recordingRunner{res: Result{Stdout: "up to date"}}

	if _, err := Update(r); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if r.inv.Exe != "skills" {
		t.Errorf("exe = %q, want skills", r.inv.Exe)
	}
	// The binding policy: always-explicit flags, never a bare `skills
	// update` whose interactive defaults could change under fleet.
	want := []string{"update", "-g", "-y"}
	if !reflect.DeepEqual(r.inv.Args, want) {
		t.Errorf("args = %v, want %v", r.inv.Args, want)
	}
	if len(r.inv.Env) == 0 {
		t.Error("env is empty — the child would lose PATH and HOME")
	}
}

func TestUpdateSurfacesAFailedRunWithItsOutput(t *testing.T) {
	// The failure the policy is built for: stdin closed, the CLI hits an
	// unexpected prompt, reads EOF, and exits non-zero with prose fleet
	// must pass through untouched.
	r := &recordingRunner{
		res: Result{Stdout: "updating...\n", Stderr: "? install prompt\n"},
		err: errors.New("exit status 1"),
	}

	res, err := Update(r)
	if err == nil {
		t.Fatal("Update() succeeded on a failed run, want error")
	}

	var cliErr *Error
	if !errors.As(err, &cliErr) {
		t.Fatalf("Update() error = %T, want *Error", err)
	}
	want := "updating...\n? install prompt"
	if cliErr.Output != want {
		t.Errorf("output = %q, want %q", cliErr.Output, want)
	}
	if res.Stdout != "updating...\n" || res.Stderr != "? install prompt\n" {
		t.Errorf("result = %+v, want the raw streams preserved", res)
	}
}

func TestUpdateSuccessCarriesNoError(t *testing.T) {
	r := &recordingRunner{res: Result{Stdout: "done"}}

	res, err := Update(r)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if res.Stdout != "done" {
		t.Errorf("stdout = %q, want done", res.Stdout)
	}
}

func TestUpdatePipesStdinClosed(t *testing.T) {
	// The child reads its stdin while it runs, so the stub reads where
	// the child would: inside Run, before Update returns and reclaims
	// the read end.
	var inv Invocation
	var readErr error
	runner := RunnerFunc(func(i Invocation) (Result, error) {
		inv = i
		_, readErr = i.Stdin.Read(make([]byte, 1))
		return Result{}, nil
	})

	if _, err := Update(runner); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Stdin must be a closed pipe, not fleet's own stdin: an unexpected
	// prompt reads EOF and fails fast instead of hanging.
	if inv.Stdin == nil {
		t.Fatal("stdin is nil — the child would inherit fleet's stdin")
	}
	if !errors.Is(readErr, io.EOF) {
		t.Errorf("child stdin read error = %v, want EOF", readErr)
	}
}
