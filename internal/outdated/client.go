package outdated

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultAPIBase = "https://api.github.com"
	// requestTimeout bounds one tree lookup so an unreachable network
	// cannot hang `fleet skill ls`.
	requestTimeout = 10 * time.Second
)

// HTTPClient is the real TreeClient, speaking GitHub's REST v3 trees API:
// GET /repos/{owner}/{repo}/git/trees/{ref}?recursive=1 — one cheap call
// per repo. Conditional requests (If-None-Match) are honored: GitHub
// answers 304 and does not charge the rate limit for them. When
// GITHUB_TOKEN or GH_TOKEN is set it is sent as a bearer token, raising
// the anonymous limit from 60 to 5000 requests per hour.
type HTTPClient struct {
	// HTTP is the transport; nil means a default client with the package
	// request timeout.
	HTTP *http.Client
	// BaseURL overrides the API base for tests. Empty means api.github.com.
	BaseURL string
}

// NewHTTPClient builds a client against the real GitHub API (or an
// overridden base URL for tests).
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{BaseURL: baseURL}
}

// apiError is what a tree lookup reports when GitHub answers but not with
// a tree: an HTTP status the caller cannot recover from. The message is
// written for humans — it surfaces verbatim in Check's warnings.
type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("GitHub API returned %d: %s", e.status, e.message)
}

func (c *HTTPClient) FetchTree(ctx context.Context, owner, repo, ref, ifNoneMatch string) (TreeResponse, error) {
	if ref == "" {
		ref = "HEAD" // always resolves the default branch in one call
	}
	base := c.BaseURL
	if base == "" {
		base = defaultAPIBase
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1",
		strings.TrimSuffix(base, "/"), url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(ref))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return TreeResponse{}, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "fleet")
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	if token := apiToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return TreeResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusOK:
		return parseTree(resp)
	case resp.StatusCode == http.StatusNotModified:
		return TreeResponse{NotModified: true, ETag: resp.Header.Get("ETag")}, nil
	case resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0":
		return TreeResponse{}, &apiError{status: resp.StatusCode, message: "rate limit exhausted (fleet sends GITHUB_TOKEN/GH_TOKEN when set, which raises the anonymous limit)"}
	default:
		return TreeResponse{}, &apiError{status: resp.StatusCode, message: http.StatusText(resp.StatusCode)}
	}
}

func parseTree(resp *http.Response) (TreeResponse, error) {
	var body struct {
		SHA       string      `json:"sha"`
		Truncated bool        `json:"truncated"`
		Tree      []TreeEntry `json:"tree"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return TreeResponse{}, fmt.Errorf("decoding GitHub tree response: %w", err)
	}
	// Large repos truncate recursive trees; the caller warns and treats
	// missing folders as unknown rather than trusting a partial tree.
	out := TreeResponse{ETag: resp.Header.Get("ETag"), RootSHA: body.SHA, Truncated: body.Truncated, Entries: body.Tree}
	if out.Entries == nil {
		out.Entries = []TreeEntry{}
	}
	return out, nil
}

// apiToken reads the explicit token environment the skills CLI itself
// uses. Stored GitHub CLI credentials are deliberately not extracted.
func apiToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}
