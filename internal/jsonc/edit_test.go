package jsonc

import (
	"strings"
	"testing"
)

func TestParseRenderRoundTripsVerbatim(t *testing.T) {
	src := `{
		// header comment
		"$schema": "https://opencode.ai/config.json", // trailing note
		"model": "gpt-5",
		"permission": {
			"skill": {
				"*": "allow",
				/* block
				   comment */
				"git-*": "deny",
			},
		},
		// dangling before close
	}`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := doc.Render(); got != src {
		t.Errorf("Render() =\n%q\nwant\n%q", got, src)
	}
}

func TestSetReplacesValueInPlaceKeepingComments(t *testing.T) {
	src := `{
  "model": "gpt-5", // keep me
  "permission": {
    "skill": {
      "tdd": "allow", // was allowed
      "git-*": "deny",
    },
  },
}`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	skill := doc.RootObject().Obj("permission").Obj("skill")
	skill.Set("tdd", `"deny"`)

	want := `{
  "model": "gpt-5", // keep me
  "permission": {
    "skill": {
      "tdd": "deny", // was allowed
      "git-*": "deny",
    },
  },
}`
	if got := doc.Render(); got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestSetAppendsNewKeyMatchingIndent(t *testing.T) {
	src := "{\n  \"model\": \"gpt-5\"\n}"
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.RootObject().Set("permission", `{"skill": {"tdd": "deny"}}`)

	want := "{\n  \"model\": \"gpt-5\",\n  \"permission\": {\"skill\": {\"tdd\": \"deny\"}}\n}"
	if got := doc.Render(); got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestSetIntoInlineObjectStaysInline(t *testing.T) {
	doc, err := Parse("{}")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.RootObject().Set("model", `"gpt-5"`)
	if got, want := doc.Render(), `{"model": "gpt-5"}`; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestSetEscapesKeysAndValues(t *testing.T) {
	doc, err := Parse("{}")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.RootObject().Set("a\"b", `"x\"y"`)
	if got, want := doc.Render(), `{"a\"b": "x\"y"}`; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestSetTwiceReplacesNotDuplicates(t *testing.T) {
	doc, err := Parse(`{"a": 1}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.RootObject().Set("b", "2")
	doc.RootObject().Set("b", "3")
	if got, want := doc.Render(), `{"a": 1, "b": 3}`; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestDeleteRemovesEntryAndNormalizesComma(t *testing.T) {
	src := `{
  "a": 1,
  "b": 2,
  "c": 3
}`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !doc.RootObject().Delete("b") {
		t.Fatal("Delete(b) = false, want true")
	}
	want := `{
  "a": 1,
  "c": 3
}`
	if got := doc.Render(); got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestDeleteLastEntryKeepsJSONCStyleDropsStrictComma(t *testing.T) {
	// JSONC files keep their trailing-comma style; strict documents must
	// not gain one from a deletion.
	src := `{"a": 1, "b": 2,}`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.RootObject().Delete("b")
	if got, want := doc.Render(), `{"a": 1,}`; got != want {
		t.Errorf("JSONC Render() = %q, want %q", got, want)
	}
	if _, err := Parse(doc.Render()); err != nil {
		t.Errorf("Render() output not re-parseable: %v", err)
	}

	sdoc, err := ParseStrict(`{"a": 1, "b": 2}`)
	if err != nil {
		t.Fatalf("ParseStrict() error = %v", err)
	}
	sdoc.RootObject().Delete("b")
	if got, want := sdoc.Render(), `{"a": 1}`; got != want {
		t.Errorf("strict Render() = %q, want %q", got, want)
	}
	if _, err := ParseStrict(sdoc.Render()); err != nil {
		t.Errorf("strict Render() output not strict JSON: %v", err)
	}
}

func TestDeleteKeepsCommentsBeforeFollowingEntry(t *testing.T) {
	src := `{
  "a": 1,
  // b is here for reasons
  "b": 2,
  // c matters
  "c": 3,
}`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.RootObject().Delete("b")
	want := `{
  "a": 1,
  // c matters
  "c": 3,
}`
	if got := doc.Render(); got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestDeleteMissingKeyIsNoop(t *testing.T) {
	doc, err := Parse(`{"a": 1}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.RootObject().Delete("missing") {
		t.Error("Delete(missing) = true, want false")
	}
	if got, want := doc.Render(), `{"a": 1}`; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestArrayAppendMatchesIndentAndFixesComma(t *testing.T) {
	src := `{
  "permissions": [
    {"action": "skill", "resource": "*", "effect": "allow"}
  ]
}`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	arr := doc.RootObject().Array("permissions")
	arr.Append(`{"action": "skill", "resource": "tdd", "effect": "deny"}`)

	want := `{
  "permissions": [
    {"action": "skill", "resource": "*", "effect": "allow"},
    {"action": "skill", "resource": "tdd", "effect": "deny"}
  ]
}`
	if got := doc.Render(); got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestArrayDeleteItem(t *testing.T) {
	src := `{
  "skills": [
    "./team",
    "-skills/tdd/SKILL.md",
    "./more",
  ],
}`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	arr := doc.RootObject().Array("skills")
	arr.Delete(1)
	want := `{
  "skills": [
    "./team",
    "./more",
  ],
}`
	if got := doc.Render(); got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestArrayDeleteOnlyItemLeavesEmptyArray(t *testing.T) {
	doc, err := Parse(`{"skills": ["-skills/tdd/SKILL.md"]}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.RootObject().Array("skills").Delete(0)
	if got, want := doc.Render(), `{"skills": []}`; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestArrayItemInspection(t *testing.T) {
	src := `{"skills": ["a", /* note */ "b"]}`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	arr := doc.RootObject().Array("skills")
	if arr.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", arr.Len())
	}
	if s, ok := arr.StringItem(0); !ok || s != "a" {
		t.Errorf("StringItem(0) = %q, %v; want \"a\", true", s, ok)
	}
	if arr.ObjectItem(0) != nil {
		t.Error("ObjectItem(0) should be nil for a string item")
	}
}

func TestKVsReturnSourceOrderAndRawValues(t *testing.T) {
	doc, err := Parse(`{"z": 1, "a": "x"}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	kvs := doc.RootObject().KVs()
	want := []KV{{Key: "z", RawValue: "1"}, {Key: "a", RawValue: `"x"`}}
	if len(kvs) != len(want) || kvs[0] != want[0] || kvs[1] != want[1] {
		t.Errorf("KVs() = %v, want %v (source order)", kvs, want)
	}
}

func TestParseStrictRejectsCommentsButAcceptsStringsContainingSlashes(t *testing.T) {
	if _, err := ParseStrict(`{"a": 1 // note}`); err == nil {
		t.Error("ParseStrict accepted a comment")
	}
	if _, err := ParseStrict(`{"url": "https://example.com/*x*"}`); err != nil {
		t.Errorf("ParseStrict rejected a string containing // and /*: %v", err)
	}
	if _, err := ParseStrict(`{"a": 1,}`); err == nil {
		t.Error("ParseStrict accepted a trailing comma")
	}
}

func TestParseRejectsBrokenInput(t *testing.T) {
	for _, src := range []string{
		`{`,
		`{"a" 1}`,
		`{"a": 1} trailing`,
		``,
		`   `,
		`[1, 2] garbage`,
		`{"a": unterminated"}`,
	} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", src)
		}
	}
}

func TestParseNonObjectRootHasNoRootObject(t *testing.T) {
	doc, err := Parse("[1]")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.RootObject() != nil {
		t.Error("RootObject() should be nil for an array root")
	}
}

func TestHasCommentIgnoresStrings(t *testing.T) {
	if hasComment(`"https://x" /* real */`) == false {
		t.Error("hasComment missed a real block comment")
	}
	if hasComment(`"not // a comment"`) {
		t.Error("hasComment flagged a string containing //")
	}
	if hasComment(`"escaped \" still // fine"`) {
		t.Error("hasComment flagged a string with an escaped quote")
	}
}

func TestRenderPreservesTrailingNewlineAndWhitespace(t *testing.T) {
	for _, src := range []string{"{\"a\": 1}\n", "{\n  \"a\": 1\n}\n\n", "[1]  "} {
		doc, err := Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", src, err)
		}
		if got := doc.Render(); got != src {
			t.Errorf("Render() = %q, want %q", got, src)
		}
	}
}

func TestRenderPreservesEmptyArrayAndObject(t *testing.T) {
	doc, err := Parse(`{"a": {}, "b": []}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got, want := doc.Render(), `{"a": {}, "b": []}`; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestAppendIntoEmptyMultilineArray(t *testing.T) {
	src := "{\n  \"skills\": [\n  ]\n}"
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.RootObject().Array("skills").Append(`"-skills/tdd/SKILL.md"`)
	want := "{\n  \"skills\": [\n    \"-skills/tdd/SKILL.md\"\n  ]\n}"
	if got := doc.Render(); got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestSetIntoEmptyMultilineObject(t *testing.T) {
	src := "{\n\t\"model\": \"x\",\n\t// stays\n}"
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	// Delete the only entry, then append: comments before '}' survive.
	doc.RootObject().Delete("model")
	doc.RootObject().Set("permission", `{"skill": {"tdd": "deny"}}`)
	if !strings.Contains(doc.Render(), "// stays") {
		t.Errorf("Render() lost the trailing comment:\n%s", doc.Render())
	}
}
