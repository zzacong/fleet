// Skill-source wiring: the write side of custom-skill discovery. The
// harnesses that take extra skill-discovery paths in their own config —
// opencode's `skills` config and pi's `skills` array — implement
// SourceWiring so the fleet repo's skills/ directory can be pointed at.
// Wiring follows the shape the config file already uses: each dialect's
// decoder silently skips the other's shape, so writing the shape the file
// speaks is the only way to be heard. A file with no `skills` key yet gets
// the V1 shape, the same safe default the permission writes use.

package harness

import (
	"fmt"
	"path/filepath"

	"github.com/zacong/fleet/internal/jsonc"
	"github.com/zacong/fleet/internal/paths"
)

// WireResult reports one harness's wiring of the repo skills dir.
// Harnesses with nothing to say (already wired) are omitted by
// WireSkillSource.
type WireResult struct {
	Harness Harness
	// Changed reports whether the config was written.
	Changed bool
	// Where names the config location the path went into: "skills.paths"
	// (opencode V1 object) or "skills" (opencode V2 array and pi).
	Where string
}

// SourceWiring is the write side of custom-skill discovery, implemented by
// harnesses whose config can point at extra skill directories.
type SourceWiring interface {
	// WireSkillSource adds dir as an extra skill-discovery source.
	// Idempotent: an already-wired dir changes nothing and reports
	// Changed=false.
	WireSkillSource(dir string) (WireResult, error)
}

// WireSkillSource wires dir into every installed harness that supports
// extra skill sources (opencode, pi). Harnesses without such a config, or
// with nothing left to change, are omitted.
func WireSkillSource(p *paths.Paths, dir string) ([]WireResult, error) {
	var results []WireResult
	for _, a := range All(p) {
		w, ok := a.(SourceWiring)
		if !ok || !a.Installed() {
			continue
		}
		res, err := w.WireSkillSource(dir)
		if err != nil {
			return nil, fmt.Errorf("wire %s skill source: %w", a.Harness(), err)
		}
		if res.Changed {
			results = append(results, res)
		}
	}
	return results, nil
}

// WireSkillSource implements SourceWiring: the dir goes into the `skills`
// config in the shape the file already uses.
func (a *OpenCodeAdapter) WireSkillSource(dir string) (WireResult, error) {
	src, err := readConfig(a.home.OpenCodeConfig())
	if err != nil {
		return WireResult{}, err
	}
	doc, err := parseJSONConfig(src)
	if err != nil {
		return WireResult{}, fmt.Errorf("parse %s: %w", filepath.Base(a.home.OpenCodeConfig()), err)
	}
	root := doc.RootObject()
	if root == nil {
		return WireResult{}, fmt.Errorf("%s must contain a JSON object", filepath.Base(a.home.OpenCodeConfig()))
	}

	where := "skills"
	switch {
	case !root.Has("skills"):
		// No skills config yet. detectDialectDoc only sees the permission
		// markers here (the skills key is absent): a V2 file gets the flat
		// array its decoder reads, everything else the V1 object — the
		// safe default for files without V2 markers.
		if detectDialectDoc(root) == "v2" {
			root.Set("skills", fmt.Sprintf("[%s]", jsonc.Quote(dir)))
		} else {
			root.Set("skills", fmt.Sprintf(`{"paths": [%s]}`, jsonc.Quote(dir)))
			where = "skills.paths"
		}
	default:
		if arr := root.Array("skills"); arr != nil {
			if arrayHasString(arr, dir) {
				return WireResult{Harness: OpenCode}, nil
			}
			arr.Append(jsonc.Quote(dir))
		} else if obj := root.Obj("skills"); obj != nil {
			where = "skills.paths"
			paths := obj.Array("paths")
			if obj.Has("paths") && paths == nil {
				return WireResult{}, fmt.Errorf("%s: \"skills.paths\" must be an array", filepath.Base(a.home.OpenCodeConfig()))
			}
			if paths != nil && arrayHasString(paths, dir) {
				return WireResult{Harness: OpenCode}, nil
			}
			if paths == nil {
				obj.Set("paths", fmt.Sprintf("[%s]", jsonc.Quote(dir)))
			} else {
				paths.Append(jsonc.Quote(dir))
			}
		} else {
			return WireResult{}, fmt.Errorf("%s: \"skills\" must be an object or an array", filepath.Base(a.home.OpenCodeConfig()))
		}
	}

	if err := writeIfConfigChanged(a.home.OpenCodeConfig(), src, renderConfig(src, doc)); err != nil {
		return WireResult{}, err
	}
	return WireResult{Harness: OpenCode, Changed: true, Where: where}, nil
}

// WireSkillSource implements SourceWiring: the dir joins pi's `skills`
// array as a plain path entry — the same array exclusions live in, but a
// plain path only adds discovery, never disables anything.
func (a *PiAdapter) WireSkillSource(dir string) (WireResult, error) {
	src, err := readConfig(a.home.PiSettings())
	if err != nil {
		return WireResult{}, err
	}
	doc, err := parseStrictJSONConfig(src)
	if err != nil {
		return WireResult{}, fmt.Errorf("parse %s: %w", filepath.Base(a.home.PiSettings()), err)
	}
	root := doc.RootObject()
	if root == nil {
		return WireResult{}, fmt.Errorf("%s must contain a JSON object", filepath.Base(a.home.PiSettings()))
	}

	skills := root.Array("skills")
	if root.Has("skills") && skills == nil {
		return WireResult{}, fmt.Errorf("%s: \"skills\" must be an array", filepath.Base(a.home.PiSettings()))
	}
	if skills != nil {
		if arrayHasString(skills, dir) {
			return WireResult{Harness: Pi}, nil
		}
		skills.Append(jsonc.Quote(dir))
	} else {
		root.Set("skills", fmt.Sprintf("[%s]", jsonc.Quote(dir)))
	}

	if err := writeIfConfigChanged(a.home.PiSettings(), src, renderConfig(src, doc)); err != nil {
		return WireResult{}, err
	}
	return WireResult{Harness: Pi, Changed: true, Where: "skills"}, nil
}

// arrayHasString reports whether the array holds exactly the string s.
func arrayHasString(a *jsonc.Array, s string) bool {
	for i := 0; i < a.Len(); i++ {
		if v, ok := a.StringItem(i); ok && v == s {
			return true
		}
	}
	return false
}
