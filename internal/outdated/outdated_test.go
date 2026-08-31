package outdated

import (
	"context"
	"strings"
	"testing"
)

// stubClient is the injected GitHub API seam: it records every call and
// replies from a scripted map keyed "owner/repo@ref". No network.
type stubClient struct {
	calls []string
	reply map[string]reply
}

type reply struct {
	resp TreeResponse
	err  error
}

func (s *stubClient) FetchTree(_ context.Context, owner, repo, ref, ifNoneMatch string) (TreeResponse, error) {
	key := owner + "/" + repo + "@" + ref
	s.calls = append(s.calls, key+" if-none-match:"+ifNoneMatch)
	r, ok := s.reply[key]
	if !ok {
		return TreeResponse{}, nil
	}
	return r.resp, r.err
}

func tree(entries ...TreeEntry) TreeResponse {
	return TreeResponse{Entries: entries}
}

func entry(key string, e Entry) map[string]Entry {
	return map[string]Entry{key: e}
}

func TestCheckComparesEachSkillFolderAgainstOneCallPerRepo(t *testing.T) {
	// Two skills from mattpocock/skills share one tree call: one is
	// current (hash matches), one outdated (hash differs). The third skill
	// comes from another repo and gets its own call.
	client := &stubClient{reply: map[string]reply{
		"mattpocock/skills@": {resp: tree(
			TreeEntry{Path: "skills/engineering/tdd", Type: "tree", SHA: "1111111111111111111111111111111111111111"},
			TreeEntry{Path: "skills/engineering/code-review", Type: "tree", SHA: "2222222222222222222222222222222222222222"},
		)},
		"vercel/agent-skills@": {resp: tree(
			TreeEntry{Path: "skills/deploy-vercel", Type: "tree", SHA: "1111111111111111111111111111111111111111"},
		)},
	}}
	entries := map[string]Entry{
		"tdd":           {Source: "mattpocock/skills", SourceType: "github", SkillPath: "skills/engineering/tdd/SKILL.md", Hash: "1111111111111111111111111111111111111111"},
		"code-review":   {Source: "mattpocock/skills", SourceType: "github", SkillPath: "skills/engineering/code-review/SKILL.md", Hash: "1111111111111111111111111111111111111111"},
		"deploy-vercel": {Source: "vercel/agent-skills", SourceType: "github", SkillPath: "skills/deploy-vercel/SKILL.md", Hash: "1111111111111111111111111111111111111111"},
	}

	got := Check(context.Background(), client, entries)

	if got.Statuses["tdd"] != StatusCurrent {
		t.Errorf("tdd = %q, want current (hash matches the repo's folder tree SHA)", got.Statuses["tdd"])
	}
	if got.Statuses["code-review"] != StatusOutdated {
		t.Errorf("code-review = %q, want outdated (repo's folder hash moved on)", got.Statuses["code-review"])
	}
	if got.Statuses["deploy-vercel"] != StatusCurrent {
		t.Errorf("deploy-vercel = %q, want current", got.Statuses["deploy-vercel"])
	}
	if n := len(client.calls); n != 2 {
		t.Errorf("three skills from two repos must make exactly two API calls, calls = %v", client.calls)
	}
}

func TestCheckFetchesEachPinnedRefSeparately(t *testing.T) {
	// Same repo, two refs: each ref has its own tree, so the group — and
	// the API call — is per (source, ref), not per repo alone.
	client := &stubClient{reply: map[string]reply{
		"mattpocock/skills@v1": {resp: tree(TreeEntry{Path: "skills/tdd", Type: "tree", SHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})},
		"mattpocock/skills@v2": {resp: tree(TreeEntry{Path: "skills/tdd", Type: "tree", SHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"})},
	}}
	entries := map[string]Entry{
		"pinned-v1": {Source: "mattpocock/skills", SourceType: "github", SkillPath: "skills/tdd/SKILL.md", Hash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Ref: "v1"},
		"pinned-v2": {Source: "mattpocock/skills", SourceType: "github", SkillPath: "skills/tdd/SKILL.md", Hash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Ref: "v2"},
	}

	got := Check(context.Background(), client, entries)

	if got.Statuses["pinned-v1"] != StatusCurrent || got.Statuses["pinned-v2"] != StatusCurrent {
		t.Errorf("statuses = %q/%q, want both current against their own ref", got.Statuses["pinned-v1"], got.Statuses["pinned-v2"])
	}
	if n := len(client.calls); n != 2 {
		t.Errorf("two refs need two calls, calls = %v", client.calls)
	}
}

func TestCheckExtractsTheFolderHashTheWayTheSkillsCLIDoes(t *testing.T) {
	// The lockfile records a tree SHA for the skill FOLDER. The check
	// derives that folder from skillPath: strip the SKILL.md suffix
	// (case-insensitive), accept root-level skills, and survive
	// backslashed paths.
	client := &stubClient{reply: map[string]reply{
		"o/r@": {resp: TreeResponse{
			RootSHA: "cccccccccccccccccccccccccccccccccccccccc",
			Entries: []TreeEntry{
				{Path: "skills/Curated Skill", Type: "tree", SHA: "dddddddddddddddddddddddddddddddddddddddd"},
			},
		}},
	}}
	entries := map[string]Entry{
		"subfolder": {Source: "o/r", SourceType: "github", SkillPath: "skills/Curated Skill/SKILL.md", Hash: "dddddddddddddddddddddddddddddddddddddddd"},
		"root":      {Source: "o/r", SourceType: "github", SkillPath: "SKILL.md", Hash: "cccccccccccccccccccccccccccccccccccccccc"},
		"backslash": {Source: "o/r", SourceType: "github", SkillPath: `skills\Curated Skill\SKILL.md`, Hash: "dddddddddddddddddddddddddddddddddddddddd"},
		"suffix":    {Source: "o/r", SourceType: "github", SkillPath: "skills/Curated Skill/skill.md", Hash: "dddddddddddddddddddddddddddddddddddddddd"},
	}

	got := Check(context.Background(), client, entries)

	for _, key := range []string{"subfolder", "root", "backslash", "suffix"} {
		if got.Statuses[key] != StatusCurrent {
			t.Errorf("%s = %q, want current (folder hash resolved the same way the skills CLI records it)", key, got.Statuses[key])
		}
	}
	if n := len(client.calls); n != 1 {
		t.Errorf("one repo, one call, calls = %v", client.calls)
	}
}

func TestCheckReportsFolderMissingFromTreeAsUnknown(t *testing.T) {
	// A folder absent from the tree (moved, deleted upstream, or cut off by
	// tree truncation) cannot prove anything: unknown, never a guess.
	client := &stubClient{reply: map[string]reply{
		"o/r@": {resp: tree(TreeEntry{Path: "skills/other", Type: "tree", SHA: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"})},
	}}
	entries := map[string]Entry{
		"gone": {Source: "o/r", SourceType: "github", SkillPath: "skills/gone/SKILL.md", Hash: "85f3ba59f22c16de988e36b781ae46d0d9098f94"},
	}

	got := Check(context.Background(), client, entries)

	if got.Statuses["gone"] != StatusUnknown {
		t.Errorf("gone = %q, want unknown", got.Statuses["gone"])
	}
	if len(got.Warnings) != 0 {
		t.Errorf("a missing folder is a normal answer, not a failure; warnings = %v", got.Warnings)
	}
}

func TestCheckAcceptsGitHubURLFormSource(t *testing.T) {
	// Some lockfiles record the full clone URL as the source; the check
	// reads owner/repo out of it instead of giving up.
	client := &stubClient{reply: map[string]reply{
		"o/r@": {resp: tree(TreeEntry{Path: "skills/tdd", Type: "tree", SHA: hashOld})},
	}}
	entries := map[string]Entry{
		"url-form": {Source: "https://github.com/o/r.git", SourceType: "github", SkillPath: "skills/tdd/SKILL.md", Hash: hashOld},
	}

	got := Check(context.Background(), client, entries)

	if got.Statuses["url-form"] != StatusCurrent {
		t.Errorf("url-form = %q, want current (source parsed as owner/repo)", got.Statuses["url-form"])
	}
	if len(client.calls) != 1 || client.calls[0] != "o/r@ if-none-match:" {
		t.Errorf("calls = %v, want one call against owner o, repo r", client.calls)
	}
}

func TestCheckDegradesToUnknownWhenTheAPIFails(t *testing.T) {
	// A repo whose check fails yields unknown for its skills, one visible
	// warning, and zero effect on other repos. Two skills from the failed
	// repo still cost only one call.
	client := &stubClient{reply: map[string]reply{
		"dead/repo@":  {err: &apiError{status: 403, message: "rate limit exhausted"}},
		"alive/repo@": {resp: tree(TreeEntry{Path: "skills/fine", Type: "tree", SHA: "1111111111111111111111111111111111111111"})},
	}}
	entries := map[string]Entry{
		"dead-one": {Source: "dead/repo", SourceType: "github", SkillPath: "skills/one/SKILL.md", Hash: "1111111111111111111111111111111111111111"},
		"dead-two": {Source: "dead/repo", SourceType: "github", SkillPath: "skills/two/SKILL.md", Hash: "1111111111111111111111111111111111111111"},
		"alive":    {Source: "alive/repo", SourceType: "github", SkillPath: "skills/fine/SKILL.md", Hash: "1111111111111111111111111111111111111111"},
	}

	got := Check(context.Background(), client, entries)

	for _, key := range []string{"dead-one", "dead-two"} {
		if got.Statuses[key] != StatusUnknown {
			t.Errorf("%s = %q, want unknown on API failure", key, got.Statuses[key])
		}
	}
	if got.Statuses["alive"] != StatusCurrent {
		t.Errorf("alive = %q, want current (one broken repo must not poison the rest)", got.Statuses["alive"])
	}
	if n := len(client.calls); n != 2 {
		t.Errorf("one call per repo even when one fails, calls = %v", client.calls)
	}
	if len(got.Warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one for the failed repo", got.Warnings)
	}
	if w := got.Warnings[0]; !strings.Contains(w, "dead/repo") || !strings.Contains(w, "rate limit exhausted") {
		t.Errorf("warning should name the repo and the cause, got %q", w)
	}
}

func TestCheckWarnsOnUnparseableSource(t *testing.T) {
	// v1 checks github.com only; a source fleet cannot read as owner/repo
	// is a visible unknown, not a guessed one.
	client := &stubClient{}
	entries := map[string]Entry{
		"odd":  {Source: "not-a-repo", SourceType: "github", SkillPath: "skills/x/SKILL.md", Hash: "1111111111111111111111111111111111111111"},
		"ghes": {Source: "ghe.corp:owner/repo", SourceType: "github", SkillPath: "skills/x/SKILL.md", Hash: "1111111111111111111111111111111111111111"},
	}

	got := Check(context.Background(), client, entries)

	if got.Statuses["odd"] != StatusUnknown || got.Statuses["ghes"] != StatusUnknown {
		t.Errorf("statuses = %q/%q, want unknown for both", got.Statuses["odd"], got.Statuses["ghes"])
	}
	if len(client.calls) != 0 {
		t.Errorf("unparseable sources must not reach the API, calls = %v", client.calls)
	}
	if len(got.Warnings) != 2 {
		t.Fatalf("warnings = %v, want one per unparseable source", got.Warnings)
	}
}

func TestCheckWarnsWhenGitHubTruncatesTheTree(t *testing.T) {
	// Recursive trees of very large repos come back truncated; folders cut
	// off are unknown, and the user hears why. Folders that did survive are
	// still compared normally.
	client := &stubClient{reply: map[string]reply{
		"o/huge@": {resp: TreeResponse{
			Truncated: true,
			Entries:   []TreeEntry{{Path: "skills/survivor", Type: "tree", SHA: "1111111111111111111111111111111111111111"}},
		}},
	}}
	entries := map[string]Entry{
		"survivor": {Source: "o/huge", SourceType: "github", SkillPath: "skills/survivor/SKILL.md", Hash: "1111111111111111111111111111111111111111"},
		"cut-off":  {Source: "o/huge", SourceType: "github", SkillPath: "skills/cut-off/SKILL.md", Hash: "1111111111111111111111111111111111111111"},
	}

	got := Check(context.Background(), client, entries)

	if got.Statuses["survivor"] != StatusCurrent {
		t.Errorf("survivor = %q, want current", got.Statuses["survivor"])
	}
	if got.Statuses["cut-off"] != StatusUnknown {
		t.Errorf("cut-off = %q, want unknown (missing from a truncated tree)", got.Statuses["cut-off"])
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "truncated") {
		t.Errorf("warnings = %v, want one truncation warning for o/huge", got.Warnings)
	}
}

func TestCheckReportsNonGitHubSourcesAsUnknown(t *testing.T) {
	// Fleet never guesses: only GitHub sources are checkable at all. git,
	// local, well-known, and custom skills (no provenance at all) are
	// unknown without touching the API.
	client := &stubClient{}
	entries := map[string]Entry{
		"git-thing":   {Source: "example.com/me/thing", SourceType: "git"},
		"local-thing": {Source: "/path/to/dir", SourceType: "local"},
		"well-known":  {Source: "example.com", SourceType: "well-known"},
		"custom":      {},
	}

	got := Check(context.Background(), client, entries)

	for _, key := range []string{"git-thing", "local-thing", "well-known", "custom"} {
		if got.Statuses[key] != StatusUnknown {
			t.Errorf("%s = %q, want unknown", key, got.Statuses[key])
		}
	}
	if len(client.calls) != 0 {
		t.Errorf("non-GitHub sources must not hit the API, calls = %v", client.calls)
	}
	if len(got.Warnings) != 0 {
		t.Errorf("expected sources are not a failure, warnings = %v", got.Warnings)
	}
}

func TestCheckSkipsGitHubEntriesWithoutComparableHash(t *testing.T) {
	// skillFolderHash must be a git tree SHA (40 hex chars) and the entry
	// must know its path inside the repo; anything else cannot be compared
	// against a tree hash without guessing, so it is unknown. No API call
	// is wasted on entries that cannot produce an answer.
	client := &stubClient{reply: map[string]reply{
		// The only entry that reaches the API meets the repo's tree and
		// its folder hash matches.
		"mattpocock/skills@": {resp: tree(TreeEntry{Path: "skills/tdd", Type: "tree", SHA: "85f3ba59f22c16de988e36b781ae46d0d9098f94"})},
	}}
	entries := map[string]Entry{
		"no-hash":    {Source: "mattpocock/skills", SourceType: "github", SkillPath: "skills/tdd/SKILL.md"},
		"no-path":    {Source: "mattpocock/skills", SourceType: "github", Hash: "85f3ba59f22c16de988e36b781ae46d0d9098f94"},
		"short-hash": {Source: "mattpocock/skills", SourceType: "github", SkillPath: "skills/tdd/SKILL.md", Hash: "aaa111"},
		"weird-hash": {Source: "mattpocock/skills", SourceType: "github", SkillPath: "skills/tdd/SKILL.md", Hash: "not-even-hex-aaaaaaaaaaaaaaaaaaa"},
		"checkable":  {Source: "mattpocock/skills", SourceType: "github", SkillPath: "skills/tdd/SKILL.md", Hash: "85f3ba59f22c16de988e36b781ae46d0d9098f94"},
	}

	got := Check(context.Background(), client, entries)

	for _, key := range []string{"no-hash", "no-path", "short-hash", "weird-hash"} {
		if got.Statuses[key] != StatusUnknown {
			t.Errorf("%s = %q, want unknown", key, got.Statuses[key])
		}
	}
	if got.Statuses["checkable"] != StatusCurrent {
		t.Errorf("checkable entry = %q, want current (its folder hash matches the tree)", got.Statuses["checkable"])
	}
	if n := len(client.calls); n != 1 {
		t.Errorf("one call for one checkable entry (all four others skipped client-side), calls = %v", client.calls)
	}
}
