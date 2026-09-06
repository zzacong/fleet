package customs

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestMoveDirRenamesWithinAFilesystem(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, filepath.Join(root, "src", "my-notes"))

	// Adopt's contract: the repo skills dir exists before the move.
	if err := os.MkdirAll(filepath.Join(root, "dst"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := moveDir(filepath.Join(root, "src", "my-notes"), filepath.Join(root, "dst", "my-notes")); err != nil {
		t.Fatal(err)
	}
	assertSkillFixture(t, t, filepath.Join(root, "dst", "my-notes"))
	if _, err := os.Stat(filepath.Join(root, "src", "my-notes")); !os.IsNotExist(err) {
		t.Errorf("the source survived the move: %v", err)
	}
}

func TestMoveDirRefusesToOverwrite(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, filepath.Join(root, "src", "my-notes"))
	writeSkillFixture(t, filepath.Join(root, "dst", "my-notes"))

	if err := moveDir(filepath.Join(root, "src", "my-notes"), filepath.Join(root, "dst", "my-notes")); err == nil {
		t.Error("moving onto an existing directory should fail")
	}
	// The existing destination is untouched by the failed move.
	assertSkillFixture(t, t, filepath.Join(root, "dst", "my-notes"))
}

func TestMoveDirOfAMissingSourceFails(t *testing.T) {
	root := t.TempDir()
	if err := moveDir(filepath.Join(root, "nope"), filepath.Join(root, "dst")); err == nil {
		t.Error("moving a missing source should fail")
	}
}

// TestCopyTree covers the cross-device fallback's machinery directly: the
// copy must carry regular files with their modes, nested directories, and
// symlinks, and must never touch the source.
func TestCopyTree(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "my-notes")
	writeSkillFixture(t, src)

	dst := filepath.Join(root, "dst", "my-notes")
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	assertSkillFixture(t, t, dst)

	// The source is intact; a failed copy never loses it.
	assertSkillFixture(t, t, src)
}

func TestCopyTreeLeavesExoticaBehind(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A named pipe is not skill content; the copy skips it without failing.
	if err := syscall.Mkfifo(filepath.Join(src, "pipe"), 0o644); err != nil {
		t.Skipf("cannot create a fifo here: %v", err)
	}

	if err := copyTree(src, filepath.Join(root, "dst")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "dst", "sub", "SKILL.md")); err != nil {
		t.Errorf("the regular file did not survive: %v", err)
	}
}

// writeSkillFixture creates a skill dir with: a top-level SKILL.md, an
// executable nested script, and a symlink to outside the dir.
func writeSkillFixture(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(dir, "SKILL.md"):          "---\nname: my-notes\ndescription: Notes.\n---\n",
		filepath.Join(dir, "scripts", "run.sh"): "#!/bin/sh\necho hi\n",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(dir, "scripts", "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../SKILL.md", filepath.Join(dir, "scripts", "linked.md")); err != nil {
		t.Fatal(err)
	}
}

func assertSkillFixture(t *testing.T, tb testing.TB, dir string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil || string(body) != "---\nname: my-notes\ndescription: Notes.\n---\n" {
		t.Fatalf("SKILL.md = %q, %v", body, err)
	}
	script, err := os.ReadFile(filepath.Join(dir, "scripts", "run.sh"))
	if err != nil || string(script) != "#!/bin/sh\necho hi\n" {
		t.Fatalf("scripts/run.sh = %q, %v", script, err)
	}
	info, err := os.Stat(filepath.Join(dir, "scripts", "run.sh"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("scripts/run.sh mode = %v, %v; want executable", info.Mode().Perm(), err)
	}
	link, err := os.Readlink(filepath.Join(dir, "scripts", "linked.md"))
	if err != nil || link != "../SKILL.md" {
		t.Fatalf("scripts/linked.md = %q, %v; want the recreated symlink", link, err)
	}
}
