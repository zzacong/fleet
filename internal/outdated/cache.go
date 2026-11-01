package outdated

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// successTTL is how long a fetched repo tree answers without touching
	// the API. Conditional revalidation (If-None-Match) after that costs no
	// rate limit on a 304, so the TTL bounds staleness, not API usage.
	successTTL = time.Hour
	// failureTTL is how long a failed check is remembered. It is shorter
	// than successTTL so a transient outage does not keep the badge in the
	// dark for a full hour.
	failureTTL = 10 * time.Minute
)

// now is injectable so tests can age the cache without sleeping.
var now = time.Now

// CachedTreeClient is a TreeClient backed by fleet's TTL cache. Fresh
// answers from the stored trees alone, so a caller can paint badges before
// any network call; Revalidate fetches even inside the TTL, for a manual
// refresh.
type CachedTreeClient interface {
	TreeClient
	// Fresh returns a stored tree that is a success within the TTL. ok is
	// false when the tree must come from the network (or was a failure).
	Fresh(owner, repo, ref string) (TreeResponse, bool)
	// Revalidate fetches the tree from the network, skipping the TTL but
	// still sending the stored ETag.
	Revalidate(ctx context.Context, owner, repo, ref string) (TreeResponse, error)
}

// NewCachingClient wraps client with a persistent TTL cache stored at
// cachePath (fleet's own config dir; never the skills CLI lockfile). Two
// separate fleet processes pointing at the same file — two `fleet skill ls`
// runs — share the cache, which is the point: repeated runs must not
// hammer the API. The cache is best-effort; an unwritable or corrupt file
// degrades to "no cache", never to a wrong answer.
//
// The cache is safe for concurrent use: its state is guarded, and a
// network fetch never runs under that guard.
func NewCachingClient(client TreeClient, cachePath string) CachedTreeClient {
	return &cachingClient{client: client, path: cachePath}
}

type cachingClient struct {
	client TreeClient
	path   string

	// mu guards loaded and cache. It is never held across a network call,
	// so a Fresh read stays instant while a fetch is in flight.
	mu     sync.Mutex
	loaded bool
	cache  treeCache
}

// treeCache is the on-disk format of fleet's repo-tree cache. It is
// fleet-owned state, versioned for forward compatibility.
type treeCache struct {
	Version int                   `json:"version"`
	Entries map[string]cacheEntry `json:"entries"`
}

type cacheEntry struct {
	// A successful lookup: the tree plus the ETag for conditional
	// revalidation.
	ETag      string      `json:"etag,omitempty"`
	FetchedAt time.Time   `json:"fetchedAt"`
	RootSHA   string      `json:"rootSha,omitempty"`
	Truncated bool        `json:"truncated,omitempty"`
	Tree      []TreeEntry `json:"tree,omitempty"`
	// A failed lookup: remembered briefly so repeated offline runs fail
	// instantly instead of timing out every time.
	FailedAt time.Time `json:"failedAt,omitempty"`
	Err      string    `json:"err,omitempty"`
}

// FetchTree serves a stored tree within its TTL, replays a recent failure,
// and otherwise asks the network with the stored ETag.
func (c *cachingClient) FetchTree(ctx context.Context, owner, repo, ref, _ string) (TreeResponse, error) {
	// The caller never passes an ETag: the cache owns the conditional
	// request, so whatever the caller cached is what gets revalidated.
	return c.fetch(ctx, owner, repo, ref, false)
}

// Revalidate goes to the network even when the stored tree is still within
// its TTL.
func (c *cachingClient) Revalidate(ctx context.Context, owner, repo, ref string) (TreeResponse, error) {
	return c.fetch(ctx, owner, repo, ref, true)
}

// Fresh answers from the store alone: a success still inside the TTL.
func (c *cachingClient) Fresh(owner, repo, ref string) (TreeResponse, bool) {
	entry, ok := c.lookup(cacheKey(owner, repo, ref))
	if !ok || entry.Err != "" || now().Sub(entry.FetchedAt) >= successTTL {
		return TreeResponse{}, false
	}
	return serve(entry), true
}

// fetch is the one path both FetchTree and Revalidate take. force skips the
// stored answer, success or failure, and goes straight to the network.
func (c *cachingClient) fetch(ctx context.Context, owner, repo, ref string, force bool) (TreeResponse, error) {
	k := cacheKey(owner, repo, ref)
	entry, ok := c.lookup(k)
	if ok && !force {
		if entry.Err == "" {
			if now().Sub(entry.FetchedAt) < successTTL {
				return serve(entry), nil
			}
		} else if now().Sub(entry.FailedAt) < failureTTL {
			// A remembered failure replays its original error so the
			// warning reads the same as a live one.
			return TreeResponse{}, errors.New(entry.Err)
		}
	}

	resp, err := c.client.FetchTree(ctx, owner, repo, ref, entry.ETag)
	if err != nil {
		c.store(k, cacheEntry{FailedAt: now(), Err: err.Error()})
		return TreeResponse{}, err
	}
	if resp.NotModified {
		// Unchanged upstream: keep the stored tree, restart the TTL.
		entry.FetchedAt = now()
		c.store(k, entry)
		return serve(entry), nil
	}
	entry = cacheEntry{
		ETag:      resp.ETag,
		FetchedAt: now(),
		RootSHA:   resp.RootSHA,
		Truncated: resp.Truncated,
		Tree:      resp.Entries,
	}
	c.store(k, entry)
	return resp, nil
}

func cacheKey(owner, repo, ref string) string {
	return owner + "/" + repo + "@" + ref
}

// serve turns a stored entry into the response Check would have gotten
// from the network.
func serve(e cacheEntry) TreeResponse {
	return TreeResponse{
		ETag:      e.ETag,
		RootSHA:   e.RootSHA,
		Truncated: e.Truncated,
		Entries:   e.Tree,
	}
}

// lookup reads one stored entry, loading the file on first use.
func (c *cachingClient) lookup(k string) (cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	entry, ok := c.cache.Entries[k]
	return entry, ok
}

// load reads the cache file once. Callers hold c.mu.
func (c *cachingClient) load() {
	if c.loaded {
		return
	}
	c.loaded = true
	c.cache = treeCache{Version: 1, Entries: map[string]cacheEntry{}}

	body, err := os.ReadFile(c.path)
	if err != nil {
		return // missing cache = cold cache
	}
	var parsed treeCache
	if json.Unmarshal(body, &parsed) != nil || parsed.Entries == nil {
		return // corrupt cache = cold cache
	}
	c.cache = parsed
}

// store merges one entry and persists the file. Write failures are
// silently ignored: the cache is an optimization, and a warning about it
// would only drown the warnings that matter.
func (c *cachingClient) store(k string, entry cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	c.cache.Entries[k] = entry

	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return
	}
	body, err := json.Marshal(c.cache)
	if err != nil {
		return
	}
	tmp := c.path + ".tmp"
	if os.WriteFile(tmp, body, 0o644) != nil {
		return
	}
	_ = os.Rename(tmp, c.path) // atomic swap; readers never see a torn file
}
