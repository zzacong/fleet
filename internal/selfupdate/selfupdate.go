// Package selfupdate detects a newer fleet binary on GitHub Releases and
// tells the user to re-run their install command. It never upgrades
// anything itself: the install doors (install.sh, npm, go install) stay
// the only writers. Every failure is silent — a missed notice costs
// nothing, a failed command costs trust.
package selfupdate

import (
	"context"
	"time"
)

// successTTL bounds notice staleness: at most one Releases lookup per day.
const successTTL = 24 * time.Hour

// failureTTL is how long a failed lookup suppresses the next attempt, so
// offline runs fail instantly instead of timing out on every command.
const failureTTL = 10 * time.Minute

// now is injectable so tests can age the cache without sleeping.
var now = time.Now

// Check reports the latest release tag when it is newer than current.
// current is buildinfo.Version ("dev" builds and "" never check — there is
// nothing to compare). A cold or stale cache triggers one Releases lookup;
// any failure (network, parse, cache write) returns ("", false) silently.
func Check(ctx context.Context, cachePath, current string, client ReleaseClient) (string, bool) {
	if normalize(current) == "" || normalize(current) == "dev" {
		return "", false
	}
	cached := load(cachePath)
	if cached.Latest != "" && now().Sub(cached.CheckedAt) < successTTL {
		if IsNewer(cached.Latest, current) {
			return cached.Latest, true
		}
		return "", false
	}
	if now().Sub(cached.FailedAt) < failureTTL {
		return "", false
	}
	latest, err := client.FetchLatest(ctx)
	if err != nil || normalize(latest) == "" {
		store(cachePath, cached.withFailure(now()))
		return "", false
	}
	store(cachePath, cached.withSuccess(latest, now()))
	if IsNewer(latest, current) {
		return latest, true
	}
	return "", false
}
