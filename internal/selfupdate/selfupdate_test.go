package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"0.2.0", "0.1.0", true},   // leading v optional on both sides
		{"v0.1.0", "0.1.0", false}, // same version, different spelling
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false}, // older latest is never newer
		{"v0.1.1", "v0.1.0", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.2", "v0.1.9", true}, // missing patch defaults to zero
		{"dev", "v0.1.0", false}, // unparsable latest never nags
		{"v0.2.0", "dev", false}, // unparsable current never nags
		{"", "v0.1.0", false},
		{"v0.2.0", "", false},
		{"banana", "v0.1.0", false},
	}
	for _, tc := range cases {
		if got := IsNewer(tc.latest, tc.current); got != tc.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

type stubClient struct {
	latest string
	err    error
	calls  int
}

func (s *stubClient) FetchLatest(context.Context) (string, error) {
	s.calls++
	return s.latest, s.err
}

func cachePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "version-check.json")
}

func TestCheckSkipsDevWithoutNetwork(t *testing.T) {
	stub := &stubClient{latest: "v9.9.9"}
	if latest, ok := Check(context.Background(), cachePath(t), "dev", stub); ok || latest != "" {
		t.Errorf("dev build checked: (%q, %v), want no notice", latest, ok)
	}
	if stub.calls != 0 {
		t.Errorf("dev build hit the network %d times, want 0", stub.calls)
	}
}

func TestCheckNoticesNewerAndCaches(t *testing.T) {
	path := cachePath(t)
	stub := &stubClient{latest: "v0.2.0"}
	latest, ok := Check(context.Background(), path, "v0.1.0", stub)
	if !ok || latest != "v0.2.0" {
		t.Fatalf("Check = (%q, %v), want (v0.2.0, true)", latest, ok)
	}
	// Second run serves the 24h cache without touching the network.
	stub.latest = "v0.3.0"
	latest, ok = Check(context.Background(), path, "v0.1.0", stub)
	if !ok || latest != "v0.2.0" {
		t.Errorf("cached Check = (%q, %v), want (v0.2.0, true)", latest, ok)
	}
	if stub.calls != 1 {
		t.Errorf("network calls = %d, want 1 (second run cached)", stub.calls)
	}
}

func TestCheckCurrentNeedsNoNotice(t *testing.T) {
	stub := &stubClient{latest: "v0.1.0"}
	if latest, ok := Check(context.Background(), cachePath(t), "v0.1.0", stub); ok || latest != "" {
		t.Errorf("current build notified: (%q, %v), want silence", latest, ok)
	}
}

func TestCheckFailureIsSilentAndBrieflyRemembered(t *testing.T) {
	path := cachePath(t)
	stub := &stubClient{err: errors.New("offline")}
	if latest, ok := Check(context.Background(), path, "v0.1.0", stub); ok || latest != "" {
		t.Fatalf("failed Check = (%q, %v), want silence", latest, ok)
	}
	// A remembered failure serves silence without retrying.
	if _, ok := Check(context.Background(), path, "v0.1.0", stub); ok {
		t.Error("failed Check retried immediately, want remembered silence")
	}
	if stub.calls != 1 {
		t.Errorf("network calls = %d, want 1 (failure remembered)", stub.calls)
	}
}

func TestCheckCorruptCacheIsColdCache(t *testing.T) {
	path := cachePath(t)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	stub := &stubClient{latest: "v0.2.0"}
	if latest, ok := Check(context.Background(), path, "v0.1.0", stub); !ok || latest != "v0.2.0" {
		t.Errorf("corrupt-cache Check = (%q, %v), want (v0.2.0, true)", latest, ok)
	}
}

func TestFetchLatestParsesTagName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/zzacong/fleet/releases/latest" {
			t.Errorf("path = %s, want releases/latest", r.URL.Path)
		}
		if ua := r.Header.Get("User-Agent"); ua != "fleet" {
			t.Errorf("User-Agent = %q, want fleet", ua)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0"}`))
	}))
	defer srv.Close()
	got, err := NewHTTPReleaseClient(srv.URL).FetchLatest(context.Background())
	if err != nil || got != "v0.2.0" {
		t.Errorf("FetchLatest = (%q, %v), want (v0.2.0, nil)", got, err)
	}
}

func TestFetchLatestRejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := NewHTTPReleaseClient(srv.URL).FetchLatest(context.Background()); err == nil {
		t.Error("FetchLatest on 403 = nil error, want an error")
	}
}

func TestRenderPlainAndBoxed(t *testing.T) {
	plain := Render("v0.1.0", "v0.2.0", false)
	for _, want := range []string{
		"fleet update available: v0.1.0 → v0.2.0",
		"curl -fsSL https://raw.githubusercontent.com/zzacong/fleet/main/install.sh | sh",
		"npm i -g @zzacong/fleet",
		"go install github.com/zzacong/fleet/cmd/fleet@latest",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("plain render missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "\x1b[") {
		t.Error("plain render contains ANSI escapes")
	}
	t.Setenv("NO_COLOR", "1")
	boxed := Render("v0.1.0", "v0.2.0", true)
	if strings.Contains(boxed, "\x1b[") {
		t.Error("NO_COLOR render contains ANSI escapes")
	}
	if !strings.Contains(boxed, "fleet update available") {
		t.Error("NO_COLOR render lost the title")
	}
}

func TestStaleCacheRevalidates(t *testing.T) {
	path := cachePath(t)
	stub := &stubClient{latest: "v0.1.0"}
	if _, ok := Check(context.Background(), path, "v0.1.0", stub); ok {
		t.Fatal("current build should not notify")
	}
	// Age the cache past the 24h TTL: the next run must hit the network.
	prev := now
	now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	t.Cleanup(func() { now = prev })
	stub.latest = "v0.2.0"
	if latest, ok := Check(context.Background(), path, "v0.1.0", stub); !ok || latest != "v0.2.0" {
		t.Errorf("stale-cache Check = (%q, %v), want (v0.2.0, true)", latest, ok)
	}
}
