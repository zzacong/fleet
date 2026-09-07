package tui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zzacong/fleet/internal/buildinfo"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/selfupdate"
)

func stubReleases(t *testing.T, tag string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `"}`))
	}))
	t.Cleanup(srv.Close)
	prev := newReleaseClient
	newReleaseClient = func(string) *selfupdate.HTTPReleaseClient {
		return selfupdate.NewHTTPReleaseClient(srv.URL)
	}
	t.Cleanup(func() { newReleaseClient = prev })
}

// TestFleetUpdateLineShowsFooterWhenNewer stamps a release version (tests
// otherwise run as dev, which never checks) and serves a newer tag with a
// cold cache: the footer line must name both versions.
func TestFleetUpdateLineShowsFooterWhenNewer(t *testing.T) {
	prevVersion := buildinfo.Version
	buildinfo.Version = "v0.1.0"
	t.Cleanup(func() { buildinfo.Version = prevVersion })
	stubReleases(t, "v0.2.0")

	got := fleetUpdateLine(paths.New(t.TempDir()))
	want := "fleet v0.1.0 → v0.2.0 available — rerun your install command"
	if got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
}

// TestFleetUpdateLineSilentOnClientError locks the launch promise: a dead
// network with a cold cache never surfaces in the TUI.
func TestFleetUpdateLineSilentOnClientError(t *testing.T) {
	prev := newReleaseClient
	newReleaseClient = func(string) *selfupdate.HTTPReleaseClient {
		return selfupdate.NewHTTPReleaseClient("http://127.0.0.1:1")
	}
	t.Cleanup(func() { newReleaseClient = prev })

	if got := fleetUpdateLine(paths.New(t.TempDir())); got != "" {
		t.Errorf("error footer = %q, want empty", got)
	}
}
