// Skill-source unwiring: the inverse of wiring.go. Removing a dropped
// collection dir from the harnesses that take extra skill-discovery paths
// in their own config — opencode's `skills` config (V1 `skills.paths`
// object shape and V2 `skills` array shape) and pi's `skills` array.
// Unwiring is idempotent: an absent dir changes nothing and is a no-op.
// Unlike wiring, unwiring never creates a missing config file: an
// installed harness with no config file yet has nothing to remove.

package harness

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zzacong/fleet/internal/jsonc"
	"github.com/zzacong/fleet/internal/paths"
)

// UnwireResult reports one harness's unwiring of the repo skills dir.
// Harnesses with nothing to say (dir absent) are omitted by
// UnwireSkillSource.
type UnwireResult struct {
	Harness Harness
	// Changed reports whether the config was written.
	Changed bool
	// Where names the config location the path came out of:
	// "skills.paths" (opencode V1 object) or "skills" (opencode V2 array
	// and pi).
	Where string
}

// SourceUnwiring is the removal side of custom-skill discovery,
// implemented by harnesses whose config can point at extra skill
// directories.
type SourceUnwiring interface {
	// UnwireSkillSource removes dir as an extra skill-discovery source.
	// Idempotent: an absent dir changes nothing and reports
	// Changed=false. A missing config file is also a no-op — it is
	// never created.
	UnwireSkillSource(dir string) (UnwireResult, error)
}

// UnwireSkillSource unwires dir from every installed harness that supports
// extra skill sources (opencode, pi). Harnesses without such a config, or
// with nothing left to change, are omitted. Uninstalled harness configs
// are never created or modified.
func UnwireSkillSource(p *paths.Paths, dir string) ([]UnwireResult, error) {
	var results []UnwireResult
	for _, a := range All(p) {
		w, ok := a.(SourceUnwiring)
		if !ok || !a.Installed() {
			continue
		}
		res, err := w.UnwireSkillSource(dir)
		if err != nil {
			return nil, fmt.Errorf("unwire %s skill source: %w", a.Harness(), err)
		}
		if res.Changed {
			results = append(results, res)
		}
	}
	return results, nil
}

// UnwireSkillSource implements SourceUnwiring: the dir comes out of the
// `skills` config in the shape the file already uses. A missing config
// file or a file with no `skills` key is a no-op.
func (a *OpenCodeAdapter) UnwireSkillSource(dir string) (UnwireResult, error) {
	src, err := readConfig(a.home.OpenCodeConfig())
	if err != nil {
		return UnwireResult{}, err
	}
	if strings.TrimSpace(src) == "" {
		return UnwireResult{Harness: OpenCode}, nil
	}
	doc, err := parseJSONConfig(src)
	if err != nil {
		return UnwireResult{}, fmt.Errorf("parse %s: %w", filepath.Base(a.home.OpenCodeConfig()), err)
	}
	root := doc.RootObject()
	if root == nil {
		return UnwireResult{}, fmt.Errorf("%s must contain a JSON object", filepath.Base(a.home.OpenCodeConfig()))
	}

	where := "skills"
	if !root.Has("skills") {
		return UnwireResult{Harness: OpenCode}, nil
	}
	if arr := root.Array("skills"); arr != nil {
		if !removeStringItem(arr, dir) {
			return UnwireResult{Harness: OpenCode}, nil
		}
	} else if obj := root.Obj("skills"); obj != nil {
		where = "skills.paths"
		paths := obj.Array("paths")
		if obj.Has("paths") && paths == nil {
			return UnwireResult{}, fmt.Errorf("%s: \"skills.paths\" must be an array", filepath.Base(a.home.OpenCodeConfig()))
		}
		if paths == nil || !removeStringItem(paths, dir) {
			return UnwireResult{Harness: OpenCode}, nil
		}
	} else {
		return UnwireResult{}, fmt.Errorf("%s: \"skills\" must be an object or an array", filepath.Base(a.home.OpenCodeConfig()))
	}

	if err := writeIfConfigChanged(a.home.OpenCodeConfig(), src, renderConfig(src, doc)); err != nil {
		return UnwireResult{}, err
	}
	return UnwireResult{Harness: OpenCode, Changed: true, Where: where}, nil
}

// UnwireSkillSource implements SourceUnwiring: the dir comes out of pi's
// `skills` array, leaving every other entry (exclusions, other sources)
// in place. A missing settings file or a file with no `skills` key is a
// no-op.
func (a *PiAdapter) UnwireSkillSource(dir string) (UnwireResult, error) {
	src, err := readConfig(a.home.PiSettings())
	if err != nil {
		return UnwireResult{}, err
	}
	if strings.TrimSpace(src) == "" {
		return UnwireResult{Harness: Pi}, nil
	}
	doc, err := parseStrictJSONConfig(src)
	if err != nil {
		return UnwireResult{}, fmt.Errorf("parse %s: %w", filepath.Base(a.home.PiSettings()), err)
	}
	root := doc.RootObject()
	if root == nil {
		return UnwireResult{}, fmt.Errorf("%s must contain a JSON object", filepath.Base(a.home.PiSettings()))
	}

	skills := root.Array("skills")
	if !root.Has("skills") {
		return UnwireResult{Harness: Pi}, nil
	}
	if skills == nil {
		return UnwireResult{}, fmt.Errorf("%s: \"skills\" must be an array", filepath.Base(a.home.PiSettings()))
	}
	if !removeStringItem(skills, dir) {
		return UnwireResult{Harness: Pi}, nil
	}

	if err := writeIfConfigChanged(a.home.PiSettings(), src, renderConfig(src, doc)); err != nil {
		return UnwireResult{}, err
	}
	return UnwireResult{Harness: Pi, Changed: true, Where: "skills"}, nil
}

// removeStringItem deletes every array item holding exactly the string s,
// reporting whether anything was removed.
func removeStringItem(a *jsonc.Array, s string) bool {
	removed := false
	for i := a.Len() - 1; i >= 0; i-- {
		if v, ok := a.StringItem(i); ok && v == s {
			a.Delete(i)
			removed = true
		}
	}
	return removed
}
