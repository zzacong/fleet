package jsonc

import (
	"encoding/json"
	"testing"
)

func TestStripRemovesLineAndBlockComments(t *testing.T) {
	in := `{
		// line comment
		"model": "gpt-5", /* block
		   comment */
		"count": 3,
	}`
	out, err := Strip(in)
	if err != nil {
		t.Fatalf("Strip() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stripped output is not valid JSON: %v\n%s", err, out)
	}
	if got["model"] != "gpt-5" || got["count"] != float64(3) {
		t.Errorf("unexpected values: %v", got)
	}
}

func TestStripRemovesTrailingCommas(t *testing.T) {
	in := `{"a": [1, 2, 3,], "b": {"c": 1,},}`
	out, err := Strip(in)
	if err != nil {
		t.Fatalf("Strip() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stripped output is not valid JSON: %v\n%s", err, out)
	}
}

func TestStripKeepsCommentMarkersInsideStrings(t *testing.T) {
	in := `{"url": "https://example.com/a//b", "note": "not /* a comment */, ok"}`
	out, err := Strip(in)
	if err != nil {
		t.Fatalf("Strip() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stripped output is not valid JSON: %v\n%s", err, out)
	}
	if got["url"] != "https://example.com/a//b" {
		t.Errorf("url altered: %q", got["url"])
	}
	if got["note"] != "not /* a comment */, ok" {
		t.Errorf("note altered: %q", got["note"])
	}
}

func TestStripHandlesEscapedQuotesInsideStrings(t *testing.T) {
	in := `{"say": "he said \"keep // this\", /* not a comment */ done"}`
	out, err := Strip(in)
	if err != nil {
		t.Fatalf("Strip() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stripped output is not valid JSON: %v\n%s", err, out)
	}
}

func TestStripPassesPlainJSONThrough(t *testing.T) {
	in := `{"a": 1}`
	out, err := Strip(in)
	if err != nil {
		t.Fatalf("Strip() error = %v", err)
	}
	if out != in {
		t.Errorf("Strip(plain JSON) = %q, want unchanged %q", out, in)
	}
}

func TestUnmarshalDecodesJSONCIntoStruct(t *testing.T) {
	in := `{
		// dialect marker
		"permissions": [
			{ "action": "skill", "resource": "*", "effect": "deny", },
		],
	}`
	var got struct {
		Permissions []struct {
			Action string `json:"action"`
		} `json:"permissions"`
	}
	if err := Unmarshal([]byte(in), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got.Permissions) != 1 || got.Permissions[0].Action != "skill" {
		t.Errorf("unexpected decode: %+v", got)
	}
}
