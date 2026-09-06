// The opencode write side: project desired enablement into
// ~/.config/opencode/opencode.jsonc through the dialect already present in
// the file. V2 auto-migrates V1 keys while V1 silently drops V2-only keys,
// so V1 is the safe default for files without V2 markers. Everything fleet
// doesn't own — comments, unknown keys, formatting, foreign rules —
// survives the read-modify-write untouched.

package harness

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zzacong/fleet/internal/jsonc"
)

// CanProject implements Adapter: opencode has a config lever in both
// dialects.
func (a *OpenCodeAdapter) CanProject() bool { return true }

// Project implements Adapter.
func (a *OpenCodeAdapter) Project(writes []SkillWrite) (WriteReport, error) {
	rep := WriteReport{}
	if err := validateNames(writes); err != nil {
		return rep, err
	}

	read, err := a.Read(writeNames(writes))
	if err != nil {
		return WriteReport{}, err
	}

	src, err := readConfig(a.home.OpenCodeConfig())
	if err != nil {
		return WriteReport{}, err
	}
	doc, err := parseJSONConfig(src)
	if err != nil {
		return WriteReport{}, fmt.Errorf("parse %s: %w", filepath.Base(a.home.OpenCodeConfig()), err)
	}
	root := doc.RootObject()
	if root == nil {
		return WriteReport{}, fmt.Errorf("%s must contain a JSON object", filepath.Base(a.home.OpenCodeConfig()))
	}

	switch detectDialectDoc(root) {
	case "v2":
		err = projectOpenCodeV2(root, writes, read, &rep)
	default:
		// V1 is the safe default for files without V2 markers (the
		// ticket's dialect-detection guard: a file whose only marker is
		// the V1-shaped skills object must be treated as V2, which
		// detectDialectDoc already does).
		err = projectOpenCodeV1(root, writes, read, &rep)
	}
	if err != nil {
		return WriteReport{}, err
	}

	flagUnmanagedOpenCode(root, writes, &rep)

	if err := writeIfConfigChanged(a.home.OpenCodeConfig(), src, renderConfig(src, doc)); err != nil {
		return WriteReport{}, err
	}
	return rep, nil
}

// flagUnmanagedOpenCode reports deny entries no write covers: exact-name
// entries for skills fleet isn't tracking, plus pattern and blanket rules
// fleet can never own. Written skills are handled by their own write, so
// the sweep skips them — and skips patterns whose effect on a written
// skill that write already reported.
func flagUnmanagedOpenCode(root *jsonc.Object, writes []SkillWrite, rep *WriteReport) {
	written := writeNameSet(writes)
	if detectDialectDoc(root) == "v2" {
		perms := root.Array("permissions")
		if perms == nil {
			return
		}
		for i := 0; i < perms.Len(); i++ {
			o := perms.ObjectItem(i)
			if o == nil {
				continue
			}
			action, okA := o.String("action")
			resource, okR := o.String("resource")
			effect, okE := o.String("effect")
			if !okA || !okR || !okE || effect != "deny" || (action != "skill" && action != "*") {
				continue
			}
			if written[resource] {
				continue // a write manages this skill
			}
			if action == "*" || isGlobPattern(resource) {
				if patternCoversWrite(resource, writes) {
					continue // the write for that skill already reported it
				}
				flagForeign(rep, fmt.Sprintf("permissions rule for %q (action %q, effect deny) is not fleet's", resource, action))
				continue
			}
			flagNotTracked(rep, resource)
		}
		return
	}

	perm := root.Obj("permission")
	if perm == nil {
		return
	}
	if shorthand, ok := perm.String("skill"); ok {
		if shorthand == "deny" {
			flagForeign(rep, `permission.skill is a blanket "deny"`)
		}
		return
	}
	skill := perm.Obj("skill")
	if skill == nil {
		return
	}
	for _, kv := range skill.KVs() {
		effect, ok := jsonString(kv.RawValue)
		if !ok || effect != "deny" || written[kv.Key] {
			continue
		}
		if isGlobPattern(kv.Key) {
			if patternCoversWrite(kv.Key, writes) {
				continue // the write for that skill already reported it
			}
			flagForeign(rep, fmt.Sprintf("permission.skill rule %q (effect deny) is not fleet's", kv.Key))
			continue
		}
		flagNotTracked(rep, kv.Key)
	}
}

// detectDialectDoc classifies the parsed config with the read side's
// semantics; a file without any marker gets V1.
func detectDialectDoc(root *jsonc.Object) string {
	switch {
	case root.Has("permissions"):
		return "v2"
	case root.Has("permission"):
		return "v1"
	case root.Has("skills"):
		return "v2"
	default:
		return "v1"
	}
}

func projectOpenCodeV1(root *jsonc.Object, writes []SkillWrite, read ReadResult, rep *WriteReport) error {
	perm := root.Obj("permission")
	if root.Has("permission") && perm == nil {
		return fmt.Errorf(`"permission" must be an object`)
	}

	// Denies to write when the permission subtree has to be created fresh;
	// generated values are opaque text, so they are built in one go.
	var fresh []string

	for _, w := range writes {
		before := read.States[w.Name]
		switch w.State {
		case StateOff:
			if before == StateOff {
				flagOpenCodeV1(perm, w.Name, rep)
				continue
			}
			if perm == nil {
				fresh = append(fresh, w.Name)
				continue
			}
			if err := denyOpenCodeV1(perm, w.Name, before, rep); err != nil {
				return err
			}
		case StateOn:
			if before != StateOff || perm == nil {
				continue
			}
			enableOpenCodeV1(perm, w.Name, rep)
		}
	}

	if perm == nil && len(fresh) > 0 {
		root.Set("permission", openCodeV1DenyMap(fresh))
		for _, name := range fresh {
			rep.Changed = append(rep.Changed, Change{Skill: name, From: read.States[name], To: StateOff})
		}
	}
	return nil
}

// openCodeV1DenyMap renders {"skill": {"a": "deny", ...}} compactly.
func openCodeV1DenyMap(names []string) string {
	var b strings.Builder
	b.WriteString(`{"skill": {`)
	for i, name := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s: \"deny\"", jsonc.Quote(name))
	}
	b.WriteString("}}")
	return b.String()
}

// denyOpenCodeV1 makes permission.skill deny name, whatever shape it
// currently has: absent (created), pattern map (exact entries replaced,
// deny appended last so "last matching rule wins" resolves to it), or
// string shorthand (converted to a map that keeps its meaning).
func denyOpenCodeV1(perm *jsonc.Object, name string, before State, rep *WriteReport) error {
	switch skill := perm.Obj("skill"); {
	case !perm.Has("skill"):
		perm.Set("skill", fmt.Sprintf("{%s: \"deny\"}", jsonc.Quote(name)))
	case skill != nil:
		for _, key := range skill.Keys() {
			if key == name {
				skill.Delete(key)
			}
		}
		skill.Set(name, `"deny"`)
	default:
		shorthand, ok := perm.String("skill")
		if !ok {
			return fmt.Errorf(`"permission.skill" must be an object or a string`)
		}
		if shorthand == "deny" {
			// A blanket deny already covers the skill; nothing to add.
			flagForeign(rep, `permission.skill is a blanket "deny"`)
			return nil
		}
		perm.Set("skill", fmt.Sprintf(`{"*": %s, %s: "deny"}`, jsonc.Quote(shorthand), jsonc.Quote(name)))
	}
	rep.Changed = append(rep.Changed, Change{Skill: name, From: before, To: StateOff})
	return nil
}

// enableOpenCodeV1 removes fleet's exact-name entries from
// permission.skill and reports any pattern rule that still denies.
func enableOpenCodeV1(perm *jsonc.Object, name string, rep *WriteReport) {
	switch skill := perm.Obj("skill"); {
	case !perm.Has("skill"):
		return
	case skill != nil:
		removed := false
		for _, key := range skill.Keys() {
			if key == name {
				skill.Delete(key)
				removed = true
			}
		}
		rule, _, found := v1Winner(v1RulesFromDoc(perm), name)
		if found && rule.effect == "deny" {
			rep.Flags = append(rep.Flags, Flag{Skill: name, Message: fmt.Sprintf("still disabled by permission.skill rule %q — left alone", rule.pattern)})
			return
		}
		if removed {
			rep.Changed = append(rep.Changed, Change{Skill: name, From: StateOff, To: StateOn})
		}
	default:
		// A string shorthand can only be "deny" here (otherwise the state
		// would read on). Rewriting the user's blanket is not fleet's call.
		rep.Flags = append(rep.Flags, Flag{Skill: name, Message: `still disabled by the permission.skill "deny" shorthand — left alone`})
	}
}

// flagOpenCodeV1 reports a deny that already holds but isn't fleet's own
// exact-name entry; exact entries that agree with the state stay silent.
func flagOpenCodeV1(perm *jsonc.Object, name string, rep *WriteReport) {
	if perm == nil {
		return
	}
	rule, exact, found := v1Winner(v1RulesFromDoc(perm), name)
	if found && rule.effect == "deny" && !exact {
		rep.Flags = append(rep.Flags, Flag{Skill: name, Message: fmt.Sprintf("still disabled by permission.skill rule %q — left alone", rule.pattern)})
	}
}

// v1RulesFromDoc extracts permission.skill as an ordered rule list, from
// either the pattern map or the string shorthand form.
func v1RulesFromDoc(perm *jsonc.Object) []skillRule {
	if !perm.Has("skill") {
		return nil
	}
	if o := perm.Obj("skill"); o != nil {
		var rules []skillRule
		for _, kv := range o.KVs() {
			effect, ok := jsonString(kv.RawValue)
			if !ok {
				continue // not a shape fleet models; the write side leaves it alone
			}
			rules = append(rules, skillRule{pattern: kv.Key, effect: effect})
		}
		return rules
	}
	if shorthand, ok := perm.String("skill"); ok {
		return []skillRule{{pattern: "*", effect: shorthand}}
	}
	return nil
}

// v1Winner returns the last rule matching name and whether that rule is an
// exact-name entry (the shape fleet writes and manages).
func v1Winner(rules []skillRule, name string) (rule skillRule, exact bool, found bool) {
	for _, r := range rules {
		if !matchPattern(r.pattern, name) {
			continue
		}
		rule, exact, found = r, r.pattern == name, true
	}
	return rule, exact, found
}

func projectOpenCodeV2(root *jsonc.Object, writes []SkillWrite, read ReadResult, rep *WriteReport) error {
	perms := root.Array("permissions")
	if root.Has("permissions") && perms == nil {
		return fmt.Errorf(`"permissions" must be an array`)
	}

	var fresh []string

	for _, w := range writes {
		before := read.States[w.Name]
		switch w.State {
		case StateOff:
			if before == StateOff {
				flagOpenCodeV2(perms, w.Name, rep)
				continue
			}
			if perms == nil {
				fresh = append(fresh, w.Name)
				continue
			}
			// Rules are evaluated last-match-wins, so appending puts the
			// deny in charge of the skill.
			perms.Append(openCodeDenyRule(w.Name))
			rep.Changed = append(rep.Changed, Change{Skill: w.Name, From: before, To: StateOff})
		case StateOn:
			if before != StateOff || perms == nil {
				continue
			}
			removed := false
			for i := perms.Len() - 1; i >= 0; i-- {
				if openCodeIsFleetDeny(perms.ObjectItem(i), w.Name) {
					perms.Delete(i)
					removed = true
				}
			}
			rule, _, found := v2Winner(perms, w.Name)
			if found && rule.effect == "deny" {
				rep.Flags = append(rep.Flags, Flag{Skill: w.Name, Message: fmt.Sprintf("still disabled by a permissions rule for %q — left alone", rule.resource)})
				continue
			}
			if removed {
				rep.Changed = append(rep.Changed, Change{Skill: w.Name, From: StateOff, To: StateOn})
			}
		}
	}

	if perms == nil && len(fresh) > 0 {
		var b strings.Builder
		b.WriteString("[")
		for i, name := range fresh {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(openCodeDenyRule(name))
		}
		b.WriteString("]")
		root.Set("permissions", b.String())
		for _, name := range fresh {
			rep.Changed = append(rep.Changed, Change{Skill: name, From: read.States[name], To: StateOff})
		}
	}
	return nil
}

// openCodeDenyRule renders fleet's V2 rule shape compactly.
func openCodeDenyRule(name string) string {
	return fmt.Sprintf(`{"action": "skill", "resource": %s, "effect": "deny"}`, jsonc.Quote(name))
}

// openCodeIsFleetDeny reports whether the parsed rule is exactly the shape
// fleet writes: three keys, action "skill", the exact resource, effect
// deny. Anything else — extra keys, patterns, wildcard actions — isn't
// fleet's to remove.
func openCodeIsFleetDeny(o *jsonc.Object, name string) bool {
	if o == nil || len(o.Keys()) != 3 {
		return false
	}
	action, okA := o.String("action")
	resource, okR := o.String("resource")
	effect, okE := o.String("effect")
	return okA && okR && okE && action == "skill" && resource == name && effect == "deny"
}

// flagOpenCodeV2 reports a deny that already holds but isn't fleet's own
// exact rule.
func flagOpenCodeV2(perms *jsonc.Array, name string, rep *WriteReport) {
	if perms == nil {
		return
	}
	rule, exact, found := v2Winner(perms, name)
	if found && rule.effect == "deny" && !exact {
		rep.Flags = append(rep.Flags, Flag{Skill: name, Message: fmt.Sprintf("still disabled by a permissions rule for %q — left alone", rule.resource)})
	}
}

// v2Winner returns the last rule matching name and whether that rule is
// exactly fleet's own deny shape for the skill.
func v2Winner(perms *jsonc.Array, name string) (rule v2Rule, exact bool, found bool) {
	for i := 0; i < perms.Len(); i++ {
		o := perms.ObjectItem(i)
		if o == nil {
			continue // generated this round, or not an object
		}
		action, okA := o.String("action")
		resource, okR := o.String("resource")
		effect, okE := o.String("effect")
		if !okA || !okR || !okE {
			continue
		}
		if (action != "skill" && action != "*") || !matchPattern(resource, name) {
			continue
		}
		rule = v2Rule{action: action, resource: resource, effect: effect}
		exact, found = openCodeIsFleetDeny(o, name), true
	}
	return rule, exact, found
}
