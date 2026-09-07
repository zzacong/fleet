package selfupdate

import (
	"strconv"
	"strings"
)

// normalize trims the version to its comparable core: surrounding space,
// one leading "v", and any pre-release or build suffix ("-rc.1", "+meta").
// "v0.2.0" and "0.2.0" compare equal; "dev" stays "dev" for the caller's
// skip check.
func normalize(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return v
}

// IsNewer reports whether latest is a higher X.Y.Z than current. Non-numeric
// segments never count as newer — a tag fleet cannot parse must not nag.
// Missing segments default to zero ("1.2" == "1.2.0").
func IsNewer(latest, current string) bool {
	l, lok := parse(normalize(latest))
	c, cok := parse(normalize(current))
	if !lok || !cok {
		return false
	}
	for i := range 3 {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) > 3 || len(parts) == 0 {
		return out, false
	}
	for i, p := range parts {
		if p == "" {
			return out, false
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
