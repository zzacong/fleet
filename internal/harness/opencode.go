package harness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/zzacong/fleet/internal/jsonc"
	"github.com/zzacong/fleet/internal/paths"
)

// OpenCode reads ~/.config/opencode/opencode.jsonc. One file, two config
// dialects: V1 `permission.skill` (string shorthand or pattern map) and V2
// beta `permissions` rules ({action, resource, effect}). Both dialects also
// configure extra skill sources: V1 `skills: {paths, urls}`, V2 `skills: [...]`.
//
// Semantics per dialect are "last matching rule wins"; a deny hides the
// skill from the catalog, ask and allow do not. opencode scans the canonical
// store natively, so without a matching deny every skill is on.
type OpenCodeAdapter struct {
	home *paths.Paths
}

// NewOpenCode builds the opencode adapter under an injected home root.
func NewOpenCode(p *paths.Paths) *OpenCodeAdapter { return &OpenCodeAdapter{home: p} }

// Harness implements Adapter.
func (a *OpenCodeAdapter) Harness() Harness { return OpenCode }

// Installed implements Adapter.
func (a *OpenCodeAdapter) Installed() bool { return isDir(a.home.OpenCodeDir()) }

// Dir implements Adapter.
func (a *OpenCodeAdapter) Dir() string { return a.home.OpenCodeDir() }

// Read implements Adapter.
func (a *OpenCodeAdapter) Read(names []string) (ReadResult, error) {
	res := ReadResult{States: onForAll(names)}

	body, err := os.ReadFile(a.home.OpenCodeConfig())
	if err != nil {
		if os.IsNotExist(err) {
			return res, nil
		}
		return ReadResult{}, err
	}

	var cfg struct {
		Permission  json.RawMessage `json:"permission"`
		Permissions []v2Rule        `json:"permissions"`
		Skills      json.RawMessage `json:"skills"`
	}
	if err := jsonc.Unmarshal(body, &cfg); err != nil {
		return ReadResult{}, fmt.Errorf("parse %s: %w", filepath.Base(a.home.OpenCodeConfig()), err)
	}

	res.Dialect = detectDialect(cfg.Permission, cfg.Permissions != nil, cfg.Skills)
	res.SkillSources = parseSkillSources(cfg.Skills)

	// V1 baseline: permission.skill, evaluated per dialect semantics. The
	// rules are parsed once: the state loop evaluates them per name, and
	// the exact denies among them feed Disables.
	var v1Rules []skillRule
	if len(cfg.Permission) > 0 {
		var perm struct {
			Skill json.RawMessage `json:"skill"`
		}
		if err := json.Unmarshal(cfg.Permission, &perm); err != nil {
			return ReadResult{}, fmt.Errorf("parse permission in %s: %w", filepath.Base(a.home.OpenCodeConfig()), err)
		}
		rules, err := parseV1SkillRules(perm.Skill)
		if err != nil {
			return ReadResult{}, fmt.Errorf("parse permission.skill in %s: %w", filepath.Base(a.home.OpenCodeConfig()), err)
		}
		v1Rules = rules
		for _, name := range names {
			if effect, matched := lastMatchingEffect(v1Rules, name); matched {
				res.States[name] = effectState(effect)
			}
		}
	}

	// V2 rules decide last: V2 auto-migrates V1 keys, so in a mixed file
	// the V2 ruleset is what the running opencode enforces.
	for _, name := range names {
		matched := false
		var effect string
		for _, rule := range cfg.Permissions {
			if rule.action != "skill" && rule.action != "*" {
				continue
			}
			if !matchPattern(rule.resource, name) {
				continue
			}
			effect = rule.effect
			matched = true
		}
		if matched {
			res.States[name] = effectState(effect)
		}
	}

	res.Disables = openCodeDisables(v1Rules, cfg.Permissions)
	return res, nil
}

// openCodeDisables collects the exact-name deny entries in the config: V1
// map keys and V2 rules with an exact resource, sorted. In a mixed file
// both dialects count — V2 migrates the V1 keys, so a V1 deny still
// disables. The string shorthand, patterns, and wildcard actions are not
// exact entries.
func openCodeDisables(v1 []skillRule, v2 []v2Rule) []string {
	seen := map[string]bool{}
	var disables []string
	add := func(name string) {
		if name == "" || isGlobPattern(name) || seen[name] {
			return
		}
		seen[name] = true
		disables = append(disables, name)
	}
	for _, rule := range v1 {
		if rule.effect == "deny" {
			add(rule.pattern)
		}
	}
	for _, rule := range v2 {
		if rule.action == "skill" && rule.effect == "deny" {
			add(rule.resource)
		}
	}
	sort.Strings(disables)
	return disables
}

type v2Rule struct {
	action   string
	resource string
	effect   string
}

func (r *v2Rule) UnmarshalJSON(data []byte) error {
	var raw struct {
		Action   string `json:"action"`
		Resource string `json:"resource"`
		Effect   string `json:"effect"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.action, r.resource, r.effect = raw.Action, raw.Resource, raw.Effect
	return nil
}

type skillRule struct {
	pattern string
	effect  string
}

// parseV1SkillRules decodes permission.skill in key order: either a string
// shorthand (one rule for every skill) or a pattern→effect map. JSON object
// key order matters ("last matching rule wins"), which map decoding loses,
// so the rules are read as an ordered token stream.
func parseV1SkillRules(raw []byte) ([]skillRule, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var shorthand string
	if err := json.Unmarshal(raw, &shorthand); err == nil {
		return []skillRule{{pattern: "*", effect: shorthand}}, nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil {
		return nil, err
	} else if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("expected an object or string, got %v", tok)
	}

	var rules []skillRule
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		pattern, _ := keyTok.(string)

		valTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		effect, _ := valTok.(string)

		if pattern != "" && effect != "" {
			rules = append(rules, skillRule{pattern: pattern, effect: effect})
		}
	}
	if _, err := dec.Token(); err != nil { // closing '}'
		return nil, err
	}
	return rules, nil
}

// lastMatchingEffect returns the effect of the last rule whose pattern
// matches the skill name.
func lastMatchingEffect(rules []skillRule, name string) (string, bool) {
	effect := ""
	matched := false
	for _, r := range rules {
		if matchPattern(r.pattern, name) {
			effect = r.effect
			matched = true
		}
	}
	return effect, matched
}

// effectState maps a permission effect to a per-skill state: only deny
// hides a skill from opencode; allow and ask leave it on.
func effectState(effect string) State {
	if effect == "deny" {
		return StateOff
	}
	return StateOn
}

// detectDialect classifies the config file. Presence of the V2 `permissions`
// array or either `skills` shape is a V2 marker; the V1 `permission` object
// marks V1. The guard case: a file whose only marker is the V1-shaped
// `skills: {paths, urls}` object must NOT read as V1 — the V2 decoder
// silently skips that shape, so writing V1 there would be lost.
func detectDialect(permission json.RawMessage, hasPermissions bool, skills json.RawMessage) string {
	switch {
	case hasPermissions:
		return "v2"
	case permission != nil:
		return "v1"
	case len(skills) > 0:
		return "v2"
	default:
		return ""
	}
}

// parseSkillSources reads the `skills` config in either dialect: the V1
// object {paths, urls} or the V2 flat array of paths and URLs.
func parseSkillSources(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}

	var flat []string
	if err := json.Unmarshal(raw, &flat); err == nil {
		return flat
	}

	var obj struct {
		Paths []string `json:"paths"`
		Urls  []string `json:"urls"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return append(obj.Paths, obj.Urls...)
}

// matchPattern reports whether a glob pattern (as used by opencode
// permission rules and pi exclusion globs) matches a skill name. An empty
// pattern never matches.
func matchPattern(pattern, name string) bool {
	if pattern == "" {
		return false
	}
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}
