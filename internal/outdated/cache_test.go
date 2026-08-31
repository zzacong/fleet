package outdated

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newStubCacheDir returns a fresh, empty cache directory for one "process".
func newStubCacheDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config", "fleet")
}

func cachePath(dir string) string { return filepath.Join(dir, "tree-cache.json") }

var (
	hashOld = "1111111111111111111111111111111111111111"
	hashNew = "2222222222222222222222222222222222222222"
)

// tddEntry is a checkable skill from a scripted repo.
func tddEntry(hash string) Entry {
	return Entry{Source: "o/r", SourceType: "github", SkillPath: "skills/tdd/SKILL.md", Hash: hash}
}

func setClock(t *testing.T, at time.Time) {
	t.Helper()
	prev := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = prev })
}

func TestCacheSparesTheAPIOnRepeatedRuns(t *testing.T) {
	dir := newStubCacheDir(t)
	setClock(t, time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))

	client := &stubClient{reply: map[string]reply{
		"o/r@": {resp: tree(TreeEntry{Path: "skills/tdd", Type: "tree", SHA: hashOld})},
	}}

	// Run 1: a cold cache means one API call.
	run1 := Check(context.Background(), NewCachingClient(client, cachePath(dir)), entry("tdd", tddEntry(hashOld)))
	if run1.Statuses["tdd"] != StatusCurrent {
		t.Fatalf("run 1: tdd = %q, want current", run1.Statuses["tdd"])
	}
	if len(client.calls) != 1 {
		t.Fatalf("run 1: calls = %v, want exactly one", client.calls)
	}

	// Run 2, a separate process later the same hour: the fresh cached tree
	// answers, and the API is not touched at all.
	run2 := Check(context.Background(), NewCachingClient(client, cachePath(dir)), entry("tdd", tddEntry(hashOld)))
	if run2.Statuses["tdd"] != StatusCurrent {
		t.Errorf("run 2: tdd = %q, want current from the cache", run2.Statuses["tdd"])
	}
	if len(client.calls) != 1 {
		t.Errorf("run 2: repeated ls must not call the API while the cache is fresh, calls = %v", client.calls)
	}
}

func TestCacheRevalidatesStaleTreesConditionally(t *testing.T) {
	dir := newStubCacheDir(t)
	base := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	setClock(t, base)

	client := &stubClient{reply: map[string]reply{
		"o/r@": {resp: TreeResponse{ETag: `"etag-1"`, Entries: []TreeEntry{{Path: "skills/tdd", Type: "tree", SHA: hashOld}}}},
	}}
	if got := Check(context.Background(), NewCachingClient(client, cachePath(dir)), entry("tdd", tddEntry(hashOld))); got.Statuses["tdd"] != StatusCurrent {
		t.Fatalf("seed run: tdd = %q, want current", got.Statuses["tdd"])
	}

	// An hour later the entry is stale: revalidate by sending the stored
	// ETag. GitHub answers 304 Not Modified, the cached tree still answers,
	// and the TTL restarts.
	setClock(t, base.Add(2*time.Hour))
	client2 := &stubClient{reply: map[string]reply{
		"o/r@": {resp: TreeResponse{NotModified: true}},
	}}
	run := Check(context.Background(), NewCachingClient(client2, cachePath(dir)), entry("tdd", tddEntry(hashOld)))
	if run.Statuses["tdd"] != StatusCurrent {
		t.Errorf("revalidated run: tdd = %q, want current served from the 304", run.Statuses["tdd"])
	}
	if len(client2.calls) != 1 || !strings.Contains(client2.calls[0], "if-none-match:"+`"etag-1"`) {
		t.Errorf("revalidation must send the stored ETag, calls = %v", client2.calls)
	}

	// Minutes after the revalidation the entry is fresh again: no call.
	client3 := &stubClient{}
	run = Check(context.Background(), NewCachingClient(client3, cachePath(dir)), entry("tdd", tddEntry(hashOld)))
	if run.Statuses["tdd"] != StatusCurrent || len(client3.calls) != 0 {
		t.Errorf("post-304 run: status = %q, calls = %v; want cached, no calls", run.Statuses["tdd"], client3.calls)
	}
}

func TestCacheReplacesTheTreeWhenRevalidationSaysModified(t *testing.T) {
	dir := newStubCacheDir(t)
	base := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	setClock(t, base)

	client := &stubClient{reply: map[string]reply{
		"o/r@": {resp: TreeResponse{ETag: `"etag-1"`, Entries: []TreeEntry{{Path: "skills/tdd", Type: "tree", SHA: hashOld}}}},
	}}
	if got := Check(context.Background(), NewCachingClient(client, cachePath(dir)), entry("tdd", tddEntry(hashOld))); got.Statuses["tdd"] != StatusCurrent {
		t.Fatalf("seed run: tdd = %q, want current", got.Statuses["tdd"])
	}

	// Upstream moved on while the cache aged: the conditional revalidation
	// returns the new tree, the badge flips, and the new tree is what the
	// next run sees.
	setClock(t, base.Add(2*time.Hour))
	client2 := &stubClient{reply: map[string]reply{
		"o/r@": {resp: TreeResponse{ETag: `"etag-2"`, Entries: []TreeEntry{{Path: "skills/tdd", Type: "tree", SHA: hashNew}}}},
	}}
	run := Check(context.Background(), NewCachingClient(client2, cachePath(dir)), entry("tdd", tddEntry(hashOld)))
	if run.Statuses["tdd"] != StatusOutdated {
		t.Fatalf("revalidated run: tdd = %q, want outdated against the fresh tree", run.Statuses["tdd"])
	}

	client3 := &stubClient{}
	run = Check(context.Background(), NewCachingClient(client3, cachePath(dir)), entry("tdd", tddEntry(hashOld)))
	if run.Statuses["tdd"] != StatusOutdated || len(client3.calls) != 0 {
		t.Errorf("next run: status = %q, calls = %v; want outdated from cache, no calls", run.Statuses["tdd"], client3.calls)
	}
}

func TestCacheRemembersFailuresBriefly(t *testing.T) {
	dir := newStubCacheDir(t)
	base := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	setClock(t, base)

	failing := &stubClient{reply: map[string]reply{
		"o/r@": {err: errors.New("dial tcp: network unreachable")},
	}}

	// A failed check is itself a cached result: an offline user's next ls
	// degrades to the same unknown instantly instead of timing out again.
	run1 := Check(context.Background(), NewCachingClient(failing, cachePath(dir)), entry("tdd", tddEntry(hashOld)))
	if run1.Statuses["tdd"] != StatusUnknown || len(run1.Warnings) != 1 {
		t.Fatalf("run 1: status = %q, warnings = %v; want unknown with one warning", run1.Statuses["tdd"], run1.Warnings)
	}
	if len(failing.calls) != 1 {
		t.Fatalf("run 1: calls = %v, want one", failing.calls)
	}

	run2 := Check(context.Background(), NewCachingClient(failing, cachePath(dir)), entry("tdd", tddEntry(hashOld)))
	if run2.Statuses["tdd"] != StatusUnknown || len(run2.Warnings) != 1 {
		t.Errorf("run 2: status = %q, warnings = %v; want the cached failure", run2.Statuses["tdd"], run2.Warnings)
	}
	if len(failing.calls) != 1 {
		t.Errorf("run 2: the failure must be served from cache, calls = %v", failing.calls)
	}

	// Failures go stale sooner than successes so a real outage does not
	// hide an outdated badge for an hour.
	setClock(t, base.Add(15*time.Minute))
	run3 := Check(context.Background(), NewCachingClient(failing, cachePath(dir)), entry("tdd", tddEntry(hashOld)))
	if run3.Statuses["tdd"] != StatusUnknown || len(failing.calls) != 2 {
		t.Errorf("run 3: status = %q, calls = %v; want a fresh attempt after the failure TTL", run3.Statuses["tdd"], failing.calls)
	}
}

func TestCacheRecoversFromCorruptFile(t *testing.T) {
	dir := newStubCacheDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath(dir), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	setClock(t, time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))

	client := &stubClient{reply: map[string]reply{
		"o/r@": {resp: tree(TreeEntry{Path: "skills/tdd", Type: "tree", SHA: hashOld})},
	}}
	got := Check(context.Background(), NewCachingClient(client, cachePath(dir)), entry("tdd", tddEntry(hashOld)))

	if got.Statuses["tdd"] != StatusCurrent || len(client.calls) != 1 {
		t.Errorf("status = %q, calls = %v; want a corrupt cache treated as empty", got.Statuses["tdd"], client.calls)
	}
}

func TestCacheToleratesUnwritableCacheDir(t *testing.T) {
	// The cache is best-effort: if it cannot be written, the check still
	// runs and the badge still answers — just without cross-run caching.
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	setClock(t, time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))

	client := &stubClient{reply: map[string]reply{
		"o/r@": {resp: tree(TreeEntry{Path: "skills/tdd", Type: "tree", SHA: hashOld})},
	}}
	got := Check(context.Background(), NewCachingClient(client, cachePath(dir)), entry("tdd", tddEntry(hashOld)))

	if got.Statuses["tdd"] != StatusCurrent || len(client.calls) != 1 {
		t.Errorf("status = %q, calls = %v; want a working check despite the unwritable cache", got.Statuses["tdd"], client.calls)
	}
}
