package selfupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// versionCache is the on-disk format of fleet's binary version check. It
// is fleet-owned state, versioned for forward compatibility, and
// best-effort: an unwritable or corrupt file degrades to "no cache",
// never to a wrong notice.
type versionCache struct {
	Version   int       `json:"version"`
	Latest    string    `json:"latest,omitempty"`
	CheckedAt time.Time `json:"checkedAt,omitempty"`
	FailedAt  time.Time `json:"failedAt,omitempty"`
}

func (c versionCache) withSuccess(latest string, at time.Time) versionCache {
	c.Latest = latest
	c.CheckedAt = at
	c.FailedAt = time.Time{}
	return c
}

func (c versionCache) withFailure(at time.Time) versionCache {
	c.FailedAt = at
	return c
}

func load(path string) versionCache {
	c := versionCache{Version: 1}
	body, err := os.ReadFile(path)
	if err != nil {
		return c // missing cache = cold cache
	}
	var parsed versionCache
	if json.Unmarshal(body, &parsed) != nil {
		return c // corrupt cache = cold cache
	}
	parsed.Version = 1
	return parsed
}

// store persists the cache. Write failures are silently ignored: the cache
// is an optimization, and a warning about it would only drown the notices
// that matter.
func store(path string, c versionCache) {
	c.Version = 1
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	body, err := json.Marshal(c)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, body, 0o644) != nil {
		return
	}
	_ = os.Rename(tmp, path) // atomic swap; readers never see a torn file
}
