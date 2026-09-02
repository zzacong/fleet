package outdated

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

// NewCachingClient wraps client with a persistent TTL cache stored at
// cachePath (fleet's own config dir; never the skills CLI lockfile). Two
// separate fleet processes pointing at the same file — two `fleet skill ls`
// runs — share the cache, which is the point: repeated runs must not
// hammer the API. The cache is best-effort; an unwritable or corrupt file
// degrades to "no cache", never to a wrong answer.
//
// FetchTree expects sequential use, which is what Check gives it: a
// concurrent caller would race the in-memory cache.
func NewCachingClient(client TreeClient, cachePath string) TreeClient {
	return &cachingClient{client: client, path: cachePath}
}

type cachingClient struct {
	client TreeClient
	path   string
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

func (c *cachingClient) FetchTree(ctx context.Context, owner, repo, ref, _ string) (TreeResponse, error) {
	// The caller never passes an ETag: the cache owns the conditional
	// request, so whatever the caller cached is what gets revalidated.
	c.load()

	key := owner + "/" + repo + "@" + ref
	entry, ok := c.cache.Entries[key]
	if ok {
		if entry.Err == "" {
			if now().Sub(entry.FetchedAt) < successTTL {
				return c.serve(entry), nil
			}
		} else if now().Sub(entry.FailedAt) < failureTTL {
			// A remembered failure replays its original error so the
			// warning reads the same as a live one.
			return TreeResponse{}, errors.New(entry.Err)
		}
	}

	resp, err := c.client.FetchTree(ctx, owner, repo, ref, entry.ETag)
	if err != nil {
		c.store(key, cacheEntry{FailedAt: now(), Err: err.Error()})
		return TreeResponse{}, err
	}
	if resp.NotModified {
		// Unchanged upstream: keep the stored tree, restart the TTL.
		entry.FetchedAt = now()
		c.store(key, entry)
		return c.serve(entry), nil
	}
	entry = cacheEntry{
		ETag:      resp.ETag,
		FetchedAt: now(),
		RootSHA:   resp.RootSHA,
		Truncated: resp.Truncated,
		Tree:      resp.Entries,
	}
	c.store(key, entry)
	return resp, nil
}

// serve turns a stored entry into the response Check would have gotten
// from the network.
func (c *cachingClient) serve(e cacheEntry) TreeResponse {
	return TreeResponse{
		ETag:      e.ETag,
		RootSHA:   e.RootSHA,
		Truncated: e.Truncated,
		Entries:   e.Tree,
	}
}

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
func (c *cachingClient) store(key string, entry cacheEntry) {
	c.cache.Entries[key] = entry

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
