package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadLockfileParsesProvenance(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".skill-lock.json")
	body := `{
	  "version": 3,
	  "skills": {
	    "tdd": {
	      "source": "mattpocock/skills",
	      "sourceType": "github",
	      "sourceUrl": "https://github.com/mattpocock/skills.git",
	      "skillPath": "skills/engineering/tdd/SKILL.md",
	      "skillFolderHash": "85f3ba59f22c16de988e36b781ae46d0d9098f94",
	      "installedAt": "2026-08-08T04:30:28.403Z",
	      "updatedAt": "2026-08-21T06:17:32.238Z"
	    }
	  }
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	lock, err := ReadLockfile(path)
	if err != nil {
		t.Fatalf("ReadLockfile() error = %v", err)
	}

	p, ok := lock["tdd"]
	if !ok {
		t.Fatalf("no entry for tdd: %+v", lock)
	}
	if p.Source != "mattpocock/skills" {
		t.Errorf("Source = %q", p.Source)
	}
	if p.SourceType != "github" {
		t.Errorf("SourceType = %q", p.SourceType)
	}
	if p.SkillPath != "skills/engineering/tdd/SKILL.md" {
		t.Errorf("SkillPath = %q", p.SkillPath)
	}
	if p.Hash != "85f3ba59f22c16de988e36b781ae46d0d9098f94" {
		t.Errorf("Hash = %q", p.Hash)
	}
	if p.InstalledAt != "2026-08-08T04:30:28.403Z" {
		t.Errorf("InstalledAt = %q", p.InstalledAt)
	}
	if p.UpdatedAt != "2026-08-21T06:17:32.238Z" {
		t.Errorf("UpdatedAt = %q", p.UpdatedAt)
	}
}

func TestReadLockfileParsesPinnedRef(t *testing.T) {
	// A skill installed from a branch or tag records the ref; the update
	// check must compare against that ref's tree, not the default branch.
	path := filepath.Join(t.TempDir(), ".skill-lock.json")
	body := `{"version": 3, "skills": {
	  "tdd": {
	    "source": "mattpocock/skills",
	    "sourceType": "github",
	    "skillPath": "skills/tdd/SKILL.md",
	    "skillFolderHash": "aaa111",
	    "ref": "v1.2.3"
	  }
	}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	lock, err := ReadLockfile(path)
	if err != nil {
		t.Fatalf("ReadLockfile() error = %v", err)
	}
	if got := lock["tdd"].Ref; got != "v1.2.3" {
		t.Errorf("Ref = %q, want %q", got, "v1.2.3")
	}
}

func TestReadLockfileJoinsBySanitizedDirectoryKey(t *testing.T) {
	// The skills CLI sanitizes folder names on disk; the lockfile is keyed by
	// the sanitized name, not the source path's basename.
	path := filepath.Join(t.TempDir(), ".skill-lock.json")
	body := `{"version": 3, "skills": {
	  "react-best-practices": {
	    "source": "vercel/agent-skills",
	    "skillPath": "skills/React! Best Practices/SKILL.md"
	  }
	}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	lock, err := ReadLockfile(path)
	if err != nil {
		t.Fatalf("ReadLockfile() error = %v", err)
	}
	p, ok := lock["react-best-practices"]
	if !ok {
		t.Fatalf("entry keyed by sanitized name missing: %+v", lock)
	}
	if p.Source != "vercel/agent-skills" {
		t.Errorf("Source = %q", p.Source)
	}
}

func TestReadLockfilePartialEntryKeepsEmptyFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".skill-lock.json")
	if err := os.WriteFile(path, []byte(`{"version": 3, "skills": {"x": {"source": "a/b"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	lock, err := ReadLockfile(path)
	if err != nil {
		t.Fatalf("ReadLockfile() error = %v", err)
	}
	p := lock["x"]
	if p.Source != "a/b" || p.Hash != "" || p.UpdatedAt != "" {
		t.Errorf("partial entry misparsed: %+v", p)
	}
}

func TestReadLockfileMissingFileYieldsNoProvenance(t *testing.T) {
	lock, err := ReadLockfile(filepath.Join(t.TempDir(), ".skill-lock.json"))
	if err != nil {
		t.Fatalf("ReadLockfile() error = %v", err)
	}
	if len(lock) != 0 {
		t.Fatalf("lock = %+v, want empty", lock)
	}
}

func TestReadLockfileMalformedFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".skill-lock.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLockfile(path); err == nil {
		t.Fatal("ReadLockfile() on malformed JSON should fail")
	}
}
