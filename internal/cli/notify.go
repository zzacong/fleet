// The fleet update notice: after every successful non-TUI verb, fleet
// checks GitHub Releases (24h cache, silent on failure) and prints one
// stderr box when the binary is older than latest. Stdout stays
// machine-readable: --json and piped runs never see it, and dev builds
// (nothing stamped to compare) never check.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/buildinfo"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/selfupdate"
)

// updateCheckTimeout bounds the Releases lookup so a stale cache costs one
// slow command per day at most, never a hang.
const updateCheckTimeout = 10 * time.Second

// fleetUpdateCheck is the Check seam: the real Releases lookup behind the
// persistent cache. Tests stub it; dev builds return silent before it.
var fleetUpdateCheck = func(ctx context.Context, cachePath, current string) (string, bool) {
	return selfupdate.Check(ctx, cachePath, current, selfupdate.NewHTTPReleaseClient(""))
}

// stderrIsTTY reports whether fleet's stderr is a terminal. The notice is
// styled (box + color) only there; pipes and captures get the same words
// as plain text. Indirect so tests can stub it.
func stderrIsTTY() bool { return stderrTTY() }

var stderrTTY = func() bool {
	info, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// maybeNotifyFleetUpdate prints the update notice to the command's stderr
// when one is due. It never fails the command: every error path is silent.
func maybeNotifyFleetUpdate(cmd *cobra.Command, p *paths.Paths) {
	if skipUpdateNotice(cmd) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
	defer cancel()
	latest, ok := fleetUpdateCheck(ctx, p.FleetVersionCheckFile(), buildinfo.Version)
	if !ok {
		return
	}
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), selfupdate.Render(buildinfo.Version, latest, stderrIsTTY()))
}

// skipUpdateNotice decides whether the notice stays quiet for this run:
// the TUI shows its own footer line (bare `fleet` on a terminal),
// completion/help never carry notices, and --json or piped stdout keeps
// stdout machine-readable.
func skipUpdateNotice(cmd *cobra.Command) bool {
	path := cmd.CommandPath()
	if path == "fleet" {
		return true // bare TUI (or its piped ls fallback, which is not a TTY)
	}
	if strings.Contains(path, "completion") || strings.HasSuffix(path, " help") || cmd.Name() == "help" {
		return true
	}
	if flagTrue(cmd, "json") {
		return true
	}
	return !stdoutIsTTY()
}

// flagTrue walks the command and its parents for a set boolean flag.
func flagTrue(cmd *cobra.Command, name string) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if f := c.Flags().Lookup(name); f != nil && f.Value.String() == "true" {
			return true
		}
		if f := c.PersistentFlags().Lookup(name); f != nil && f.Value.String() == "true" {
			return true
		}
	}
	return false
}
