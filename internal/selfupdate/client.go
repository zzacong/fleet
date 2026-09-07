package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// repoPath is the only source of truth for "latest": the same GitHub
	// Releases the install.sh door downloads from (npm stamps from the
	// same tag, so one source covers every door).
	repoPath = "/repos/zzacong/fleet/releases/latest"
	// requestTimeout bounds one lookup so an unreachable network cannot
	// hang a fleet command past the cache's failure TTL.
	requestTimeout = 10 * time.Second
)

// ReleaseClient is the injectable Releases seam: one call asks for the
// latest release tag (e.g. "v0.2.0"). Tests script it or point the HTTP
// client at httptest.
type ReleaseClient interface {
	FetchLatest(ctx context.Context) (string, error)
}

// HTTPReleaseClient speaks GitHub's REST v3 releases API. When
// GITHUB_TOKEN or GH_TOKEN is set it is sent as a bearer token, raising
// the anonymous limit from 60 to 5000 requests per hour.
type HTTPReleaseClient struct {
	// HTTP is the transport; nil means a default client with the request
	// timeout.
	HTTP *http.Client
	// BaseURL overrides the API base for tests. Empty means api.github.com.
	BaseURL string
}

// NewHTTPReleaseClient builds a client against the real GitHub API (or an
// overridden base URL for tests).
func NewHTTPReleaseClient(baseURL string) *HTTPReleaseClient {
	return &HTTPReleaseClient{BaseURL: baseURL}
}

// FetchLatest returns the latest release's tag name.
func (c *HTTPReleaseClient) FetchLatest(ctx context.Context) (string, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+repoPath, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "fleet")
	if token := apiToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decoding GitHub release response: %w", err)
	}
	if normalize(body.TagName) == "" {
		return "", fmt.Errorf("GitHub release has no tag_name")
	}
	return body.TagName, nil
}

// apiToken reads the explicit token environment the skills CLI itself
// uses. Stored GitHub CLI credentials are deliberately not extracted.
func apiToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}
