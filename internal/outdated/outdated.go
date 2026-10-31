// Package outdated computes fleet's own update-available badge. Fleet never
// runs the skills CLI for this (`check` is an undocumented alias of
// `update`); it compares each lockfile entry's skillFolderHash against the
// current GitHub tree hash of the skill folder in its source repo — one
// API call per source repo (grouped by source and ref), with conditional
// requests (ETag / If-None-Match) so revalidations cost no rate limit.
//
// The result is a tri-state: outdated, current, or unknown. Anything fleet
// would have to guess — non-GitHub sources, entries without a comparable
// tree hash, API failures — is unknown, never a guess. Fleet never writes
// the skills CLI lockfile; its own TTL cache of repo trees lives under the
// fleet config dir.
package outdated

import (
	"context"
	"maps"
	"sort"
	"strconv"
	"strings"
)

// Status is the tri-state outcome for one skill's update check.
type Status string

const (
	// StatusOutdated: the source repo's current tree hash for the skill
	// folder differs from the lockfile's skillFolderHash.
	StatusOutdated Status = "outdated"
	// StatusCurrent: the hashes match.
	StatusCurrent Status = "current"
	// StatusUnknown: the skill was not checkable — non-GitHub source, no
	// comparable hash, an API failure, or the folder vanished from the
	// tree. Never a guess.
	StatusUnknown Status = "unknown"
)

// Entry is one skill's provenance as the update check sees it. A custom
// skill (no lockfile entry) is the zero value, which is by definition
// unknown.
type Entry struct {
	// Source is the normalized repo, e.g. "mattpocock/skills".
	Source string
	// SourceType is the lockfile's sourceType; only "github" is checkable.
	SourceType string
	// SkillPath is the skill folder's path within the repo, e.g.
	// "skills/engineering/tdd/SKILL.md".
	SkillPath string
	// Ref is the branch or tag the skill was installed from; empty means
	// the repo's default branch.
	Ref string
	// Hash is the lockfile's skillFolderHash (a git tree SHA, 40 hex
	// chars, recorded by the skills CLI at install/update time).
	Hash string
}

// Report is the outcome for every entry plus the failures worth showing the
// user. Warnings are visible, not fatal: the command using them still
// exits 0.
type Report struct {
	// Statuses maps each entry's key to its Status. Every key in the input
	// has an entry here.
	Statuses map[string]Status
	// Warnings are human-readable lines (one per distinct failure), deduped
	// and deterministically ordered.
	Warnings []string
}

// TreeEntry is one path in a repo's git tree, trimmed to what hash lookup
// needs.
type TreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"` // "tree" or "blob"
	SHA  string `json:"sha"`
}

// TreeResponse is one successful tree lookup. NotModified marks a 304: the
// caller (or caching client) must supply the previously cached entries.
type TreeResponse struct {
	NotModified bool
	ETag        string
	RootSHA     string // the tree's own sha; the hash of a root-level skill folder
	Truncated   bool
	Entries     []TreeEntry
}

// TreeClient is the injectable GitHub API seam. One call asks for one
// repo's full recursive tree at a ref; ifNoneMatch carries the previously
// cached ETag so an unchanged repo answers 304 and costs no rate limit.
type TreeClient interface {
	FetchTree(ctx context.Context, owner, repo, ref, ifNoneMatch string) (TreeResponse, error)
}

// Group is one repo tree a check must fetch: every checkable entry that
// shares a source and ref, so the whole group costs one API call.
type Group struct {
	Source string
	Owner  string
	Repo   string
	Ref    string
	// Keys are the entry keys this tree decides.
	Keys []string
}

// Fetch returns one repo's tree at a ref. Check hands it the client's
// FetchTree; the TUI hands it the cache's revalidating fetch.
type Fetch func(ctx context.Context, owner, repo, ref string) (TreeResponse, error)

// Plan groups the checkable entries and warns about the ones that cannot be
// checked. Entries outside every group are unknown by definition and never
// reach a client.
func Plan(entries map[string]Entry) ([]Group, []string) {
	var groups []Group
	var warnings []string
	index := map[string]int{} // source + ref -> position in groups

	for _, key := range sortedKeys(entries) {
		e := entries[key]
		if e.SourceType != "github" {
			continue // git, local, well-known, custom: not checkable, by design
		}
		if !isTreeSHA(e.Hash) || e.SkillPath == "" {
			warnings = append(warnings, "cannot check "+key+": the lockfile entry has no comparable skillFolderHash or skillPath (reinstall the skill to populate it)")
			continue
		}
		owner, repo, err := parseOwnerRepo(e.Source)
		if err != nil {
			warnings = append(warnings, "cannot check "+key+": "+err.Error())
			continue
		}
		gk := e.Source + "\x00" + e.Ref
		i, ok := index[gk]
		if !ok {
			groups = append(groups, Group{Source: e.Source, Owner: owner, Repo: repo, Ref: e.Ref})
			i = len(groups) - 1
			index[gk] = i
		}
		groups[i].Keys = append(groups[i].Keys, key)
	}
	return groups, warnings
}

// Classify compares each member of g with the tree it was fetched as. A
// member whose folder is missing from the tree stays unknown, never a guess;
// a truncated tree adds a warning.
func Classify(g Group, entries map[string]Entry, resp TreeResponse) (map[string]Status, []string) {
	statuses := make(map[string]Status, len(g.Keys))
	var warnings []string
	if resp.Truncated {
		warnings = append(warnings, "GitHub returned a truncated tree for "+g.Source+"; skills missing from it are reported as unknown")
	}
	for _, key := range g.Keys {
		statuses[key] = StatusUnknown
		folder := folderPath(entries[key].SkillPath)
		remote := folderHash(resp, folder)
		if remote == "" {
			continue // folder absent from the tree (or the tree was truncated): unknown, not a guess
		}
		if remote == entries[key].Hash {
			statuses[key] = StatusCurrent
		} else {
			statuses[key] = StatusOutdated
		}
	}
	return statuses, warnings
}

// CheckGroup fetches g's tree through fetch and classifies its members. A
// failed fetch leaves every member unknown and warns once.
func CheckGroup(ctx context.Context, fetch Fetch, g Group, entries map[string]Entry) (map[string]Status, []string) {
	resp, err := fetch(ctx, g.Owner, g.Repo, g.Ref)
	if err != nil {
		statuses := make(map[string]Status, len(g.Keys))
		for _, key := range g.Keys {
			statuses[key] = StatusUnknown
		}
		return statuses, []string{"update check for " + g.Source + " failed: " + err.Error()}
	}
	return Classify(g, entries, resp)
}

// Check classifies every entry in one synchronous pass: one FetchTree per
// distinct (source, ref) group. Failures degrade to unknown with a warning.
func Check(ctx context.Context, client TreeClient, entries map[string]Entry) Report {
	groups, warnings := Plan(entries)
	statuses := make(map[string]Status, len(entries))
	for key := range entries {
		statuses[key] = StatusUnknown
	}
	fetch := func(ctx context.Context, owner, repo, ref string) (TreeResponse, error) {
		return client.FetchTree(ctx, owner, repo, ref, "")
	}
	seen := map[string]bool{}
	for _, w := range warnings {
		seen[w] = true
	}
	for _, g := range groups {
		got, w := CheckGroup(ctx, fetch, g, entries)
		maps.Copy(statuses, got)
		for _, msg := range w {
			seen[msg] = true
		}
	}
	return Report{Statuses: statuses, Warnings: sortedUnique(seen)}
}

// folderPath strips the SKILL.md file name from a lockfile skillPath,
// leaving the folder whose tree SHA the skills CLI recorded.
func folderPath(skillPath string) string {
	p := strings.ReplaceAll(skillPath, "\\", "/")
	if lower := strings.ToLower(p); strings.HasSuffix(lower, "/skill.md") {
		p = p[:len(p)-len("/skill.md")]
	} else if strings.HasSuffix(lower, "skill.md") {
		p = p[:len(p)-len("skill.md")]
	}
	return strings.TrimSuffix(p, "/")
}

// folderHash extracts the current tree hash for a skill folder from a repo
// tree response. An empty result means the folder could not be found; the
// caller reports unknown rather than guessing.
func folderHash(resp TreeResponse, folder string) string {
	if folder == "" {
		return resp.RootSHA // skill at the repo root: the root tree's own SHA
	}
	for _, e := range resp.Entries {
		if e.Type == "tree" && e.Path == folder {
			return e.SHA
		}
	}
	return ""
}

// isTreeSHA reports whether h looks like a git tree SHA (40 hex chars) —
// the only hash form a tree comparison can answer for. Older or unusual
// lockfile entries hold content hashes that would never match and would
// fake an "outdated" answer.
func isTreeSHA(h string) bool {
	if len(h) != 40 {
		return false
	}
	for _, c := range h {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

// parseOwnerRepo accepts "owner/repo" and common GitHub URL forms and
// returns the two segments the trees API needs. Anything else (enterprise
// hosts, single names, paths with extra segments) is an error: v1 checks
// github.com only, and an unknown shape must not become a guess.
func parseOwnerRepo(source string) (string, string, error) {
	s := strings.TrimSpace(source)
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimSuffix(s, "/")

	for _, prefix := range []string{"https://", "http://", "ssh://git@", "git@"} {
		if strings.HasPrefix(s, prefix) {
			s = s[len(prefix):]
			break
		}
	}
	for _, host := range []string{"github.com/", "github.com:"} {
		if strings.HasPrefix(s, host) {
			s = s[len(host):]
			break
		}
	}
	if strings.Contains(s, ":") {
		return "", "", &sourceError{source: source}
	}
	owner, repo, ok := strings.Cut(s, "/")
	if !ok || owner == "" || repo == "" || strings.ContainsAny(owner+repo, " \t") || strings.Contains(repo, "/") {
		return "", "", &sourceError{source: source}
	}
	return owner, repo, nil
}

type sourceError struct{ source string }

func (e *sourceError) Error() string {
	return "unparseable source " + strconv.Quote(e.source) + "; want owner/repo on github.com"
}

func sortedKeys(entries map[string]Entry) []string {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedUnique(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for w := range set {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}
