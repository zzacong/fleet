package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

// linksHome builds a home with every harness installed.
func linksHome(t *testing.T) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// linkDirs are the harness skills dirs that take managed custom-skill
// links, keyed by harness name.
func linkDirs(p *paths.Paths) map[string]string {
	return map[string]string{
		"codex":  p.CodexSkills(),
		"claude": p.ClaudeSkills(),
		"cursor": p.CursorSkills(),
		"bob":    p.BobSkills(),
	}
}

func readLink(t *testing.T, path string) string {
	t.Helper()
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("Readlink(%s): %v", path, err)
	}
	return target
}

func TestLinkCustomSkillLinksEveryLinkBasedHarnessAtTheRepo(t *testing.T) {
	p := linksHome(t)
	target := "/repo/skills/my-notes"

	res, err := LinkCustomSkill(p, "my-notes", target)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 4 {
		t.Fatalf("results = %v, want one per codex/claude/cursor/bob", res)
	}
	for _, r := range res {
		if r.Change.Action != LinkCreated {
			t.Errorf("%s action = %q, want created", r.Harness, r.Change.Action)
		}
	}

	for name, dir := range linkDirs(p) {
		if got := readLink(t, filepath.Join(dir, "my-notes")); got != target {
			t.Errorf("%s link target = %q, want %q", name, got, target)
		}
	}

	// opencode and pi discover customs through their config paths; the
	// canonical store must stay untouched (a link there would make
	// opencode/pi see the skill twice).
	if _, err := os.Lstat(filepath.Join(p.SkillsStore(), "my-notes")); !os.IsNotExist(err) {
		t.Error("a link was created inside the canonical store")
	}
	if _, err := os.Stat(p.OpenCodeConfig()); !os.IsNotExist(err) {
		t.Error("opencode config created by a link-only operation")
	}
}

func TestLinkCustomSkillRepointsStaleLinksAtTheRepo(t *testing.T) {
	p := linksHome(t)
	// The skills CLI's auto-link for an installed skill points at the
	// canonical store; after adoption that target is gone, so the managed
	// link must take over.
	if err := os.MkdirAll(p.CodexSkills(), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(p.SkillsStore(), "my-notes")
	if err := os.Symlink(stale, filepath.Join(p.CodexSkills(), "my-notes")); err != nil {
		t.Fatal(err)
	}

	res, err := LinkCustomSkill(p, "my-notes", "/repo/skills/my-notes")
	if err != nil {
		t.Fatal(err)
	}
	var repointed bool
	for _, r := range res {
		if r.Harness == Codex && r.Change.Action == LinkRepointed && r.Change.From == stale {
			repointed = true
		}
	}
	if !repointed {
		t.Errorf("codex link not repointed with its old target reported: %v", res)
	}
	if got := readLink(t, filepath.Join(p.CodexSkills(), "my-notes")); got != "/repo/skills/my-notes" {
		t.Errorf("codex link target = %q, want the repo", got)
	}
}

func TestLinkCustomSkillIsIdempotentAndNeverClobbersRealFiles(t *testing.T) {
	p := linksHome(t)
	target := "/repo/skills/my-notes"

	if _, err := LinkCustomSkill(p, "my-notes", target); err != nil {
		t.Fatal(err)
	}
	// A second run changes nothing: unchanged links are omitted.
	res, err := LinkCustomSkill(p, "my-notes", target)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Errorf("second run reported %v, want nothing", res)
	}

	// A real directory where a link would go is the user's: skipped, not
	// replaced.
	real := filepath.Join(p.ClaudeSkills(), "my-notes")
	if err := os.Remove(real); err != nil { // drop the managed link first
		t.Fatal(err)
	}
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "SKILL.md"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = LinkCustomSkill(p, "my-notes", target)
	if err != nil {
		t.Fatal(err)
	}
	var skipped bool
	for _, r := range res {
		if r.Harness == Claude && r.Change.Action == LinkSkipped && r.Change.Note != "" {
			skipped = true
		}
	}
	if !skipped {
		t.Errorf("claude real dir not skipped with a note: %v", res)
	}
	if body, err := os.ReadFile(filepath.Join(real, "SKILL.md")); err != nil || string(body) != "mine" {
		t.Errorf("the user's directory was touched: %q, %v", body, err)
	}
}

func TestLinkCustomSkillSkipsUninstalledHarnesses(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	if err := os.MkdirAll(p.CodexDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := LinkCustomSkill(p, "my-notes", "/repo/skills/my-notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Harness != Codex {
		t.Errorf("results = %v, want only codex", res)
	}
	if _, err := os.Stat(p.ClaudeSkills()); !os.IsNotExist(err) {
		t.Error("claude skills dir created although claude is not installed")
	}
}

func TestEveryLinkBasedHarnessHasTheRightSkillsDir(t *testing.T) {
	p := linksHome(t)
	want := map[string]string{
		"codex":  p.CodexSkills(),
		"claude": p.ClaudeSkills(),
		"cursor": p.CursorSkills(),
		"bob":    p.BobSkills(),
	}
	for _, a := range All(p) {
		l, ok := a.(SkillLinker)
		if a.Harness() == OpenCode || a.Harness() == Pi {
			if ok {
				t.Errorf("%s must not take managed links; it wires config paths", a.Harness())
			}
			continue
		}
		if !ok {
			t.Fatalf("%s does not implement SkillLinker", a.Harness())
		}
		// The adapter's own dir is where its link lands: point at a
		// missing target and confirm the link appears there.
		dir := want[string(a.Harness())]
		if _, err := l.LinkSkill("probe", "/repo/skills/probe"); err != nil {
			t.Fatalf("%s LinkSkill: %v", a.Harness(), err)
		}
		if got := readLink(t, filepath.Join(dir, "probe")); got != "/repo/skills/probe" {
			t.Errorf("%s link landed at the wrong dir: %q", a.Harness(), got)
		}
	}
}

// linkHome builds a home with the given harnesses installed and a canonical
// store holding one real skill ("tdd"), returning the Paths and the store.
func linkHome(t *testing.T, harnesses ...string) (*paths.Paths, string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	if err := os.MkdirAll(filepath.Join(p.SkillsStore(), "tdd"), 0o755); err != nil {
		t.Fatal(err)
	}
	dirs := map[string]string{
		"opencode": p.OpenCodeDir(),
		"pi":       p.PiDir(),
		"codex":    p.CodexDir(),
		"claude":   p.ClaudeDir(),
		"cursor":   p.CursorDir(),
		"bob":      p.BobDir(),
	}
	for _, h := range harnesses {
		if err := os.MkdirAll(dirs[h], 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p, p.SkillsStore()
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestSkillDirsCoversInstalledHarnessesWithNativeScanFlags(t *testing.T) {
	p, _ := linkHome(t, "opencode", "claude", "bob")

	dirs := SkillDirs(p)
	if len(dirs) != 3 {
		t.Fatalf("SkillDirs() = %v, want 3 installed harnesses", dirs)
	}
	want := map[string]SkillDir{
		"opencode": {Harness: OpenCode, Path: p.OpenCodeSkills(), NativeScan: true},
		"claude":   {Harness: Claude, Path: p.ClaudeSkills(), NativeScan: false},
		"bob":      {Harness: Bob, Path: p.BobSkills(), NativeScan: true},
	}
	for _, d := range dirs {
		wantD, ok := want[string(d.Harness)]
		if !ok {
			t.Errorf("unexpected SkillDir %v", d)
			continue
		}
		if d != wantD {
			t.Errorf("SkillDir for %s = %+v, want %+v", d.Harness, d, wantD)
		}
	}
}

func TestScanSkillDirClassifiesEntries(t *testing.T) {
	p, store := linkHome(t, "opencode", "claude")
	dir := p.OpenCodeSkills()
	repo := filepath.Join(p.Home, "dev", "fleet", "skills")
	// The repo skill a managed custom link would point at exists.
	if err := os.MkdirAll(filepath.Join(repo, "mine"), 0o755); err != nil {
		t.Fatal(err)
	}

	// The skills CLI's spam: absolute and relative links into the store.
	symlink(t, filepath.Join(store, "tdd"), filepath.Join(dir, "absolute"))
	symlink(t, "../../../.agents/skills/tdd", filepath.Join(dir, "relative"))
	// Redundant even when the skill was uninstalled since.
	symlink(t, filepath.Join(store, "gone"), filepath.Join(dir, "stale"))
	// A custom-skill-style link to the repo (ticket 03's managed links
	// land here): never redundant by construction.
	symlink(t, filepath.Join(repo, "mine"), filepath.Join(dir, "repo-link"))
	// A link at the store directory itself is not a per-skill link.
	symlink(t, store, filepath.Join(dir, "whole-store"))
	// Entries fleet doesn't own and must never touch.
	if err := os.MkdirAll(filepath.Join(dir, "real-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Dangling link to nowhere outside the store.
	symlink(t, filepath.Join(p.Home, "nope", "missing"), filepath.Join(dir, "dangling"))

	entries, err := ScanSkillDir(SkillDir{Harness: OpenCode, Path: dir, NativeScan: true}, store)
	if err != nil {
		t.Fatalf("ScanSkillDir() error = %v", err)
	}

	want := map[string]EntryClass{
		"absolute":    EntryRedundant,
		"relative":    EntryRedundant,
		"stale":       EntryRedundant,
		"repo-link":   EntryForeign,
		"whole-store": EntryForeign,
		"real-dir":    EntryUntracked,
		"README":      EntryUntracked,
		"dangling":    EntryBroken,
	}
	got := map[string]Entry{}
	for _, e := range entries {
		if _, dup := got[e.Name]; dup {
			t.Errorf("duplicate entry %q", e.Name)
		}
		got[e.Name] = e
		if e.Harness != OpenCode {
			t.Errorf("entry %q harness = %s, want opencode", e.Name, e.Harness)
		}
	}
	if len(entries) != len(want) {
		t.Errorf("ScanSkillDir() returned %d entries, want %d: %v", len(entries), len(want), entries)
	}
	for name, class := range want {
		e, ok := got[name]
		if !ok {
			t.Errorf("entry %q missing from scan", name)
			continue
		}
		if e.Class != class {
			t.Errorf("entry %q class = %q, want %q (entry: %+v)", name, e.Class, class, e)
		}
		if e.Reason == "" {
			t.Errorf("entry %q has no reason", name)
		}
	}

	// The "why" for a redundant link names the native scan; targets resolve
	// into the store.
	if abs := got["absolute"]; abs.Target != filepath.Join(store, "tdd") {
		t.Errorf("absolute target = %q, want %q", abs.Target, filepath.Join(store, "tdd"))
	}
	if rel := got["relative"]; rel.Target != filepath.Join(store, "tdd") {
		t.Errorf("relative target = %q, want %q", rel.Target, filepath.Join(store, "tdd"))
	}
}

func TestScanSkillDirSkipsHiddenEntries(t *testing.T) {
	p, store := linkHome(t, "codex")
	dir := p.CodexSkills()

	// Codex keeps its own reserved namespace inside its skills dir: real
	// directories and files the harness itself owns, never fleet's.
	if err := os.MkdirAll(filepath.Join(dir, ".system", "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A visible entry proves the scan still ran.
	if err := os.MkdirAll(filepath.Join(dir, "real-dir"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries, err := ScanSkillDir(SkillDir{Harness: Codex, Path: dir, NativeScan: true}, store)
	if err != nil {
		t.Fatalf("ScanSkillDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "real-dir" {
		t.Fatalf("entries = %+v, want only real-dir — hidden entries must be skipped, not classified", entries)
	}
}

func TestScanSkillDirClaudeStoreLinksAreExpectedOrBroken(t *testing.T) {
	p, store := linkHome(t, "claude")
	dir := p.ClaudeSkills()

	// Claude is not a native canonical-store reader: its links are the
	// discovery path, so a live store link is expected, not redundant.
	symlink(t, filepath.Join(store, "tdd"), filepath.Join(dir, "tdd"))
	// But a store link whose skill is gone leaves claude blind to it.
	symlink(t, filepath.Join(store, "gone"), filepath.Join(dir, "gone"))
	// A dangling link outside the store is broken too.
	symlink(t, filepath.Join(p.Home, "nope"), filepath.Join(dir, "dangling"))

	entries, err := ScanSkillDir(SkillDir{Harness: Claude, Path: dir, NativeScan: false}, store)
	if err != nil {
		t.Fatalf("ScanSkillDir() error = %v", err)
	}

	got := map[string]EntryClass{}
	for _, e := range entries {
		got[e.Name] = e.Class
	}
	want := map[string]EntryClass{
		"tdd":      EntryExpected,
		"gone":     EntryBroken,
		"dangling": EntryBroken,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("classes = %v, want %v", got, want)
	}
}

func TestScanSkillDirFollowsSymlinkedParentDirectories(t *testing.T) {
	// A link whose final target only reveals itself after resolving a
	// symlinked directory component still double-covers the store.
	p, store := linkHome(t, "cursor")
	dir := p.CursorSkills()
	alias := filepath.Join(p.Home, "skills-alias")
	if err := os.Symlink(store, alias); err != nil {
		t.Fatal(err)
	}
	symlink(t, filepath.Join(alias, "tdd"), filepath.Join(dir, "aliased"))

	entries, err := ScanSkillDir(SkillDir{Harness: Cursor, Path: dir, NativeScan: true}, store)
	if err != nil {
		t.Fatalf("ScanSkillDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Class != EntryRedundant {
		t.Errorf("entries = %+v, want one redundant entry", entries)
	}
}

func TestScanSkillDirReportsSymlinkLoopsAsBroken(t *testing.T) {
	p, store := linkHome(t, "pi")
	dir := p.PiSkills()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	symlink(t, b, a)
	symlink(t, a, b)

	entries, err := ScanSkillDir(SkillDir{Harness: Pi, Path: dir, NativeScan: true}, store)
	if err != nil {
		t.Fatalf("ScanSkillDir() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want both loop members", entries)
	}
	for _, e := range entries {
		if e.Class != EntryBroken {
			t.Errorf("entry %q class = %q, want broken", e.Name, e.Class)
		}
	}
}

func TestScanSkillDirMissingDirIsQuiet(t *testing.T) {
	p, store := linkHome(t, "codex")

	entries, err := ScanSkillDir(SkillDir{Harness: Codex, Path: p.CodexSkills(), NativeScan: true}, store)
	if err != nil {
		t.Fatalf("ScanSkillDir() error = %v", err)
	}
	if entries != nil {
		t.Errorf("entries = %+v, want none", entries)
	}
}

func TestScanSkillDirStoreMissingStillClassifiesByShape(t *testing.T) {
	// No canonical store at all: links can't be proven redundant (their
	// targets are missing), but classification stays safe — outside
	// symlinks are foreign, real dirs untracked, dangling links broken.
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	if err := os.MkdirAll(p.OpenCodeDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := p.OpenCodeSkills()
	symlink(t, filepath.Join(p.SkillsStore(), "tdd"), filepath.Join(dir, "tdd"))
	symlink(t, filepath.Join(home, "elsewhere"), filepath.Join(dir, "elsewhere"))

	entries, err := ScanSkillDir(SkillDir{Harness: OpenCode, Path: dir, NativeScan: true}, p.SkillsStore())
	if err != nil {
		t.Fatalf("ScanSkillDir() error = %v", err)
	}
	got := map[string]EntryClass{}
	for _, e := range entries {
		got[e.Name] = e.Class
	}
	// A link into the (missing) store in a native scanner is still
	// redundant: the harness sees every store skill natively, the link
	// adds nothing, and removing it is safe when the target is missing.
	if got["tdd"] != EntryRedundant {
		t.Errorf("tdd class = %q, want redundant", got["tdd"])
	}
	if got["elsewhere"] != EntryBroken {
		t.Errorf("elsewhere class = %q, want broken", got["elsewhere"])
	}
}
