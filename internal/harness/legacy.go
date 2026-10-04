// Legacy cleanup: the one-time removal of the fleet-owned entries the old
// wiring model left behind. Before the unified-link model, opencode and pi
// took a collection directory in their config and fleet wrote exact-name
// off-entries for custom skills on opencode, codex, and pi. Sync calls
// CleanLegacy before projection so those entries converge on the link
// model, and each removal is reported. Scope is strict: only entries
// resolving into a current custom home or naming a current custom skill are
// touched; everything else — including entries fleet cannot identify — is
// left alone.

package harness

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zzacong/fleet/internal/paths"
)

// LegacyScope bounds the one-time cleanup: the current custom home dirs and
// the current custom skill names. An entry outside the scope is never
// touched.
type LegacyScope struct {
	// Homes are the current custom home collection dirs: every tracked
	// collection plus the fleet-home fallback, as cleaned paths.
	Homes []string
	// IsCustom reports whether a name resolves to a current custom skill.
	IsCustom func(name string) bool
}

// hasHome reports whether entry names one of the current custom homes.
func (s LegacyScope) hasHome(entry string) bool {
	if entry == "" {
		return false
	}
	clean := filepath.Clean(entry)
	for _, home := range s.Homes {
		if home != "" && clean == filepath.Clean(home) {
			return true
		}
	}
	return false
}

// isCustomName reports whether name is a current custom skill.
func (s LegacyScope) isCustomName(name string) bool {
	return name != "" && s.IsCustom != nil && s.IsCustom(name)
}

// LegacyRemoval reports one legacy fleet-owned config entry the one-time
// cleanup removed. Kind names the entry family ("collection path" or
// "disable entry"); Entry is the removed value — the path for a collection
// path, the skill name for a disable entry.
type LegacyRemoval struct {
	Harness Harness
	Kind    string
	Entry   string
}

const (
	legacyCollectionPath = "collection path"
	legacyDisableEntry   = "disable entry"
)

// LegacyCleaner is the cleanup side of the old wiring model, implemented by
// the harnesses whose config held legacy fleet-owned entries (opencode, pi,
// codex).
type LegacyCleaner interface {
	// CleanLegacy removes this harness's legacy fleet-owned entries that
	// fall inside scope and reports each removal. A missing or empty
	// config file is a no-op; the file is never created.
	CleanLegacy(scope LegacyScope) ([]LegacyRemoval, error)
}

// CleanLegacy removes legacy fleet-owned entries from every installed
// harness that had a config lever in the old model, in harness order.
// Harnesses with nothing to remove report nothing. It fails fast on the
// first harness error.
func CleanLegacy(p *paths.Paths, scope LegacyScope) ([]LegacyRemoval, error) {
	var out []LegacyRemoval
	for _, a := range Installed(p) {
		c, ok := a.(LegacyCleaner)
		if !ok {
			continue
		}
		removed, err := c.CleanLegacy(scope)
		if err != nil {
			return nil, fmt.Errorf("clean %s legacy config: %w", a.Harness(), err)
		}
		out = append(out, removed...)
	}
	return out, nil
}

// CleanLegacy implements LegacyCleaner: it removes collection path entries
// from opencode's `skills` config (the V1 `skills.paths` object and the V2
// flat array) and fleet-shape exact-name deny rules from `permission.skill`
// (V1) and `permissions` (V2) that name a current custom skill. User
// source paths, non-fleet rule shapes, and untracked names stay.
func (a *OpenCodeAdapter) CleanLegacy(scope LegacyScope) ([]LegacyRemoval, error) {
	src, err := readConfig(a.home.OpenCodeConfig())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(src) == "" {
		return nil, nil
	}
	doc, err := parseJSONConfig(src)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(a.home.OpenCodeConfig()), err)
	}
	root := doc.RootObject()
	if root == nil {
		return nil, fmt.Errorf("%s must contain a JSON object", filepath.Base(a.home.OpenCodeConfig()))
	}

	var removed []LegacyRemoval

	// Collection paths: the V2 flat array, or the V1 skills.paths object.
	// Any other skills shape is not fleet's and is skipped.
	if root.Has("skills") {
		switch {
		case root.Array("skills") != nil:
			arr := root.Array("skills")
			for i := arr.Len() - 1; i >= 0; i-- {
				s, ok := arr.StringItem(i)
				if !ok || !scope.hasHome(s) {
					continue
				}
				arr.Delete(i)
				removed = append(removed, LegacyRemoval{Harness: OpenCode, Kind: legacyCollectionPath, Entry: s})
			}
		case root.Obj("skills") != nil:
			if paths := root.Obj("skills").Array("paths"); paths != nil {
				for i := paths.Len() - 1; i >= 0; i-- {
					s, ok := paths.StringItem(i)
					if !ok || !scope.hasHome(s) {
						continue
					}
					paths.Delete(i)
					removed = append(removed, LegacyRemoval{Harness: OpenCode, Kind: legacyCollectionPath, Entry: s})
				}
			}
		}
	}

	// V1 exact-name deny entries in permission.skill: a key naming a custom
	// skill whose effect is exactly "deny". The string shorthand is a
	// blanket, not an exact entry.
	if perm := root.Obj("permission"); perm != nil {
		if skill := perm.Obj("skill"); skill != nil {
			for _, kv := range skill.KVs() {
				if !scope.isCustomName(kv.Key) {
					continue
				}
				if effect, ok := jsonString(kv.RawValue); !ok || effect != "deny" {
					continue
				}
				if skill.Delete(kv.Key) {
					removed = append(removed, LegacyRemoval{Harness: OpenCode, Kind: legacyDisableEntry, Entry: kv.Key})
				}
			}
		}
	}

	// V2 fleet-shape deny rules in permissions: exactly the three-key shape
	// fleet writes, selecting a custom skill.
	if perms := root.Array("permissions"); perms != nil {
		for i := perms.Len() - 1; i >= 0; i-- {
			o := perms.ObjectItem(i)
			if o == nil {
				continue
			}
			name, ok := o.String("resource")
			if !ok || !scope.isCustomName(name) || !openCodeIsFleetDeny(o, name) {
				continue
			}
			perms.Delete(i)
			removed = append(removed, LegacyRemoval{Harness: OpenCode, Kind: legacyDisableEntry, Entry: name})
		}
	}

	if len(removed) == 0 {
		return nil, nil
	}
	if err := writeIfConfigChanged(a.home.OpenCodeConfig(), src, renderConfig(src, doc)); err != nil {
		return nil, err
	}
	return removed, nil
}

// CleanLegacy implements LegacyCleaner: it removes collection path entries
// and `-skills/<name>/SKILL.md` exact exclusions from pi's `skills` array
// when they resolve into a current custom home or name a current custom
// skill. User source paths and !glob exclusions are never touched.
func (a *PiAdapter) CleanLegacy(scope LegacyScope) ([]LegacyRemoval, error) {
	src, err := readConfig(a.home.PiSettings())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(src) == "" {
		return nil, nil
	}
	doc, err := parseStrictJSONConfig(src)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(a.home.PiSettings()), err)
	}
	root := doc.RootObject()
	if root == nil {
		return nil, fmt.Errorf("%s must contain a JSON object", filepath.Base(a.home.PiSettings()))
	}

	var removed []LegacyRemoval
	if skills := root.Array("skills"); skills != nil {
		for i := skills.Len() - 1; i >= 0; i-- {
			s, ok := skills.StringItem(i)
			if !ok {
				continue
			}
			if scope.hasHome(s) {
				skills.Delete(i)
				removed = append(removed, LegacyRemoval{Harness: Pi, Kind: legacyCollectionPath, Entry: s})
				continue
			}
			if m := piExactExclusion.FindStringSubmatch(s); m != nil && scope.isCustomName(m[1]) {
				skills.Delete(i)
				removed = append(removed, LegacyRemoval{Harness: Pi, Kind: legacyDisableEntry, Entry: m[1]})
			}
		}
	}

	if len(removed) == 0 {
		return nil, nil
	}
	if err := writeIfConfigChanged(a.home.PiSettings(), src, renderConfig(src, doc)); err != nil {
		return nil, err
	}
	return removed, nil
}

// CleanLegacy implements LegacyCleaner: it removes fleet-shape
// [[skills.config]] blocks — a simple table with a name and enabled = false
// — that select a current custom skill. Path selectors, extra keys, and
// enabled = true blocks are not fleet's off-entry shape and stay.
func (a *CodexAdapter) CleanLegacy(scope LegacyScope) ([]LegacyRemoval, error) {
	src, err := readConfig(a.home.CodexConfig())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(src) == "" {
		return nil, nil
	}
	lines := strings.Split(src, "\n")
	blocks := codexParseBlocks(lines)

	var removed []LegacyRemoval
	// Blocks are visited last-first so the line indexes of the ones before
	// them stay valid.
	for i := len(blocks) - 1; i >= 0; i-- {
		b := blocks[i]
		if !b.simple || b.name == "" || b.enabled != "false" || !scope.isCustomName(b.name) {
			continue
		}
		lines = append(lines[:b.start], lines[b.end:]...)
		lines = codexTidySeam(lines, b.start)
		removed = append(removed, LegacyRemoval{Harness: Codex, Kind: legacyDisableEntry, Entry: b.name})
	}

	if len(removed) == 0 {
		return nil, nil
	}
	if err := writeIfChanged(a.home.CodexConfig(), src, strings.Join(lines, "\n")); err != nil {
		return nil, err
	}
	return removed, nil
}
