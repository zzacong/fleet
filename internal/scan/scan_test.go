package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanStoreListsEverySkillDirectoryWithFrontmatter(t *testing.T) {
	store := filepath.Join(t.TempDir(), ".agents", "skills")
	writeSkill(t, store, "tdd", "---\nname: tdd\ndescription: Red-green-refactor workflow for strict TDD.\n")
	writeSkill(t, store, "git-helper", "---\nname: git-helper\ndescription: Wraps common git flows.\n")

	skills, err := ScanStore(store)
	if err != nil {
		t.Fatalf("ScanStore() error = %v", err)
	}

	if len(skills) != 2 {
		t.Fatalf("ScanStore() returned %d skills, want 2: %+v", len(skills), skills)
	}
	byName := map[string]Skill{}
	for _, s := range skills {
		byName[s.Name] = s
	}
	if got := byName["tdd"].Description; got != "Red-green-refactor workflow for strict TDD." {
		t.Errorf("tdd description = %q", got)
	}
	if got := byName["git-helper"].Description; got != "Wraps common git flows." {
		t.Errorf("git-helper description = %q", got)
	}
}

func TestScanStoreSkipsEntriesWithoutSKILLFile(t *testing.T) {
	store := filepath.Join(t.TempDir(), ".agents", "skills")
	writeSkill(t, store, "real", "---\nname: real\ndescription: Real skill.\n")
	if err := os.MkdirAll(filepath.Join(store, "empty-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "loose-file.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}

	skills, err := ScanStore(store)
	if err != nil {
		t.Fatalf("ScanStore() error = %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "real" {
		t.Fatalf("ScanStore() = %+v, want only [real]", skills)
	}
}

func TestScanStoreFallsBackToDirectoryNameWhenFrontmatterHasNoName(t *testing.T) {
	store := filepath.Join(t.TempDir(), ".agents", "skills")
	writeSkill(t, store, "nameless", "---\ndescription: Only a description.\n")
	writeSkill(t, store, "bare", "# Just markdown, no frontmatter.\n")

	skills, err := ScanStore(store)
	if err != nil {
		t.Fatalf("ScanStore() error = %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("ScanStore() = %+v, want 2 skills", skills)
	}
	byName := map[string]string{}
	for _, s := range skills {
		byName[s.Name] = s.Description
	}
	if _, ok := byName["nameless"]; !ok {
		t.Errorf("skill with nameless frontmatter not reported under its directory name: %+v", skills)
	}
	if _, ok := byName["bare"]; !ok {
		t.Errorf("skill without frontmatter not reported under its directory name: %+v", skills)
	}
}

func TestScanStoreParsesQuotedAndMultiLineDescriptions(t *testing.T) {
	store := filepath.Join(t.TempDir(), ".agents", "skills")
	writeSkill(t, store, "quoted", "---\nname: quoted\ndescription: \"Has: a colon and \\\"quotes\\\".\"\n")
	writeSkill(t, store, "folded", "---\nname: folded\ndescription: >-\n  First part\n  second part.\n")

	skills, err := ScanStore(store)
	if err != nil {
		t.Fatalf("ScanStore() error = %v", err)
	}
	byName := map[string]string{}
	for _, s := range skills {
		byName[s.Name] = s.Description
	}
	if got := byName["quoted"]; got != `Has: a colon and "quotes".` {
		t.Errorf("quoted description = %q", got)
	}
	if got := byName["folded"]; got != "First part second part." {
		t.Errorf("folded description = %q", got)
	}
}

func TestScanStoreMissingDirectoryReturnsNoSkills(t *testing.T) {
	skills, err := ScanStore(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("ScanStore() error = %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("ScanStore() = %+v, want empty", skills)
	}
}

func TestParseFrontmatterHandlesCRLFAndUnknownKeys(t *testing.T) {
	body := "---\r\nname: crlf-skill\r\nlicense: MIT\r\ndescription: Handles CRLF.\r\n---\r\n\r\n# Body\r\n"
	got := parseFrontmatter(body)
	if got.name != "crlf-skill" {
		t.Errorf("name = %q, want crlf-skill", got.name)
	}
	if got.description != "Handles CRLF." {
		t.Errorf("description = %q, want 'Handles CRLF.'", got.description)
	}
}

func writeSkill(t *testing.T, store, dir, content string) {
	t.Helper()
	path := filepath.Join(store, dir, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
