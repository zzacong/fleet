package outdated

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPClientFetchesTheRecursiveTree(t *testing.T) {
	var gotPath, gotAccept, gotAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAccept, gotAgent = r.URL.Path, r.Header.Get("Accept"), r.Header.Get("User-Agent")
		w.Header().Set("ETag", `"abc123"`)
		_, _ = w.Write([]byte(`{
			"sha": "roothash",
			"truncated": false,
			"tree": [
				{"path": "skills", "type": "tree", "sha": "sha-skills"},
				{"path": "skills/tdd", "type": "tree", "sha": "sha-tdd"},
				{"path": "skills/tdd/SKILL.md", "type": "blob", "sha": "sha-blob"}
			]
		}`))
	}))
	defer server.Close()

	resp, err := NewHTTPClient(server.URL).FetchTree(context.Background(), "o", "r", "", "")
	if err != nil {
		t.Fatalf("FetchTree() error = %v", err)
	}

	if gotPath != "/repos/o/r/git/trees/HEAD" {
		t.Errorf("request path = %q, want the trees API for HEAD (the default branch, one call)", gotPath)
	}
	if gotAccept != "application/vnd.github.v3+json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	if gotAgent != "fleet" {
		t.Errorf("User-Agent = %q", gotAgent)
	}
	if resp.ETag != `"abc123"` || resp.RootSHA != "roothash" {
		t.Errorf("ETag = %q, RootSHA = %q", resp.ETag, resp.RootSHA)
	}
	if len(resp.Entries) != 3 || resp.Entries[1].Path != "skills/tdd" || resp.Entries[1].SHA != "sha-tdd" {
		t.Errorf("entries = %+v", resp.Entries)
	}
	if resp.NotModified {
		t.Error("a 200 is not a 304")
	}
}

func TestHTTPClientSendsIfNoneMatchAndReportsNotModified(t *testing.T) {
	var gotIfNoneMatch string
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	resp, err := NewHTTPClient(server.URL).FetchTree(context.Background(), "o", "r", "main", `"etag-7"`)
	if err != nil {
		t.Fatalf("FetchTree() error = %v", err)
	}

	if !called || gotIfNoneMatch != `"etag-7"` {
		t.Errorf("If-None-Match = %q (called = %v), want the cached ETag so unchanged repos cost no rate limit", gotIfNoneMatch, called)
	}
	if !resp.NotModified {
		t.Error("FetchTree() should report NotModified on a 304")
	}
}

func TestHTTPClientExplainsRateLimiting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	_, err := NewHTTPClient(server.URL).FetchTree(context.Background(), "o", "r", "", "")
	if err == nil {
		t.Fatal("FetchTree() on a rate-limited response should fail")
	}
	if !strings.Contains(err.Error(), "rate limit") || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("error = %v; it should tell the user what happened and how to fix it", err)
	}
}

func TestHTTPClientFailsOnOtherStatuses(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))

		_, err := NewHTTPClient(server.URL).FetchTree(context.Background(), "o", "r", "", "")
		if err == nil {
			t.Errorf("FetchTree() on HTTP %d should fail", status)
		} else if !strings.Contains(err.Error(), http.StatusText(status)) {
			t.Errorf("error = %v, want it to name HTTP %d", err, status)
		}
		server.Close()
	}
}

func TestHTTPClientSendsTheConfiguredToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "tok-123")

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"sha": "x", "tree": []}`))
	}))
	defer server.Close()

	if _, err := NewHTTPClient(server.URL).FetchTree(context.Background(), "o", "r", "", ""); err != nil {
		t.Fatalf("FetchTree() error = %v", err)
	}
	if gotAuth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want the GITHUB_TOKEN bearer header (5000 req/hr instead of 60)", gotAuth)
	}
}

func TestHTTPClientPrefersGITHUBTokenOverGHToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "first")
	t.Setenv("GH_TOKEN", "second")

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"sha": "x", "tree": []}`))
	}))
	defer server.Close()

	if _, err := NewHTTPClient(server.URL).FetchTree(context.Background(), "o", "r", "", ""); err != nil {
		t.Fatalf("FetchTree() error = %v", err)
	}
	if gotAuth != "Bearer first" {
		t.Errorf("Authorization = %q, want GITHUB_TOKEN to win", gotAuth)
	}
}
