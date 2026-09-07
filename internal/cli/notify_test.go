package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/paths"
)

func stubUpdateCheck(t *testing.T, latest string, ok bool, calls *int) {
	t.Helper()
	prev := fleetUpdateCheck
	fleetUpdateCheck = func(context.Context, string, string) (string, bool) {
		*calls++
		return latest, ok
	}
	t.Cleanup(func() { fleetUpdateCheck = prev })
}

func stubTTYs(t *testing.T, stdout, stderr bool) {
	t.Helper()
	stdoutTTY = func() bool { return stdout }
	stderrTTY = func() bool { return stderr }
	t.Cleanup(func() {
		stdoutTTY = func() bool { return false }
		stderrTTY = func() bool { return false }
	})
}

func skillLsCmd(p *paths.Paths) *cobra.Command {
	root := NewRoot(p)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"skill", "ls"})
	return root
}

func TestUpdateNoticeSkippedWhenPiped(t *testing.T) {
	p := toggleHome(t)
	stubTTYs(t, false, false)
	var calls int
	stubUpdateCheck(t, "v9.9.9", true, &calls)
	root := skillLsCmd(p)
	if err := root.Execute(); err != nil {
		t.Fatalf("skill ls: %v", err)
	}
	if calls != 0 {
		t.Errorf("piped run checked %d times, want 0", calls)
	}
}

func TestUpdateNoticeSkippedForJSON(t *testing.T) {
	p := toggleHome(t)
	stubTTYs(t, true, true)
	var calls int
	stubUpdateCheck(t, "v9.9.9", true, &calls)
	root := NewRoot(p)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"skill", "ls", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("skill ls --json: %v", err)
	}
	if calls != 0 {
		t.Errorf("--json run checked %d times, want 0", calls)
	}
}

func TestUpdateNoticePrintedToStderrOnTTY(t *testing.T) {
	p := toggleHome(t)
	stubTTYs(t, true, false) // stderr piped: plain words, no box
	var calls int
	stubUpdateCheck(t, "v9.9.9", true, &calls)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"skill", "ls"})
	if err := root.Execute(); err != nil {
		t.Fatalf("skill ls: %v", err)
	}
	if calls != 1 {
		t.Fatalf("TTY run checked %d times, want 1", calls)
	}
	if !strings.Contains(errOut.String(), "fleet update available") {
		t.Errorf("stderr missing notice:\n%s", errOut.String())
	}
	if strings.Contains(out.String(), "fleet update available") {
		t.Error("stdout carries the notice; it must stay machine-readable")
	}
}

func TestUpdateNoticeSilentWhenCurrent(t *testing.T) {
	p := toggleHome(t)
	stubTTYs(t, true, true)
	var calls int
	stubUpdateCheck(t, "", false, &calls)
	errOut := &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(errOut)
	root.SetArgs([]string{"skill", "doctor"})
	if err := root.Execute(); err != nil {
		t.Fatalf("skill doctor: %v", err)
	}
	if strings.Contains(errOut.String(), "fleet update available") {
		t.Errorf("current build notified:\n%s", errOut.String())
	}
}

func TestUpdateNoticeSkippedForCompletion(t *testing.T) {
	p := toggleHome(t)
	stubTTYs(t, true, true)
	var calls int
	stubUpdateCheck(t, "v9.9.9", true, &calls)
	root := NewRoot(p)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"completion", "bash"})
	if err := root.Execute(); err != nil {
		t.Fatalf("completion: %v", err)
	}
	if calls != 0 {
		t.Errorf("completion checked %d times, want 0", calls)
	}
}

func TestUpdateNoticeSkippedForVersionFlag(t *testing.T) {
	p := toggleHome(t)
	stubTTYs(t, true, true)
	var calls int
	stubUpdateCheck(t, "v9.9.9", true, &calls)
	root := NewRoot(p)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("--version: %v", err)
	}
	if calls != 0 {
		t.Errorf("--version checked %d times, want 0", calls)
	}
}
