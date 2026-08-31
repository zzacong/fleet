// Package jsonc turns JSONC (JSON with comments and trailing commas, the
// dialect opencode configs use) into plain JSON so encoding/json can parse
// it. It strips only what JSON cannot represent and leaves everything else,
// including string contents, byte for byte.
package jsonc

import "encoding/json"

// Unmarshal parses JSONC data into v.
func Unmarshal(data []byte, v any) error {
	out, err := Strip(string(data))
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(out), v)
}

// Strip removes // and /* */ comments and trailing commas from JSONC text,
// leaving string contents untouched.
func Strip(in string) (string, error) {
	out := make([]byte, 0, len(in))
	inString := false
	escaped := false

	b := []byte(in)
	for i := 0; i < len(b); i++ {
		c := b[i]

		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}

		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			i += 2
			for i < len(b) && b[i] != '\n' {
				i++
			}
			if i < len(b) {
				out = append(out, '\n') // keep the newline so line numbers survive
			}
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			i += 2
			for i+1 < len(b) && (b[i] != '*' || b[i+1] != '/') {
				i++
			}
			i++ // consume the '*' of the closing '*/'; the '/' is dropped by the loop
		case c == ',':
			out = append(out, c)
			// Drop the comma when only whitespace separates it from the
			// next '}' or ']' (a trailing comma).
			j := i + 1
			for j < len(b) && isSpace(b[j]) {
				j++
			}
			if j < len(b) && (b[j] == '}' || b[j] == ']') {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, c)
		}
	}
	return string(out), nil
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
