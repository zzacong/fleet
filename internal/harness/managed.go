package harness

import (
	"fmt"
	"slices"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

// Managed returns the adapters fleet acts on: the installed harnesses the
// user has not hidden, in All's order. Every write and check path uses it;
// Installed stays detection only, and All stays the full list for listing.
// A config that cannot be read is an error, never a silent "nothing hidden".
func Managed(p *paths.Paths) ([]Adapter, error) {
	hidden, err := HiddenNames(p)
	if err != nil {
		return nil, err
	}
	var out []Adapter
	for _, a := range Installed(p) {
		if !slices.Contains(hidden, string(a.Harness())) {
			out = append(out, a)
		}
	}
	return out, nil
}

// IsHidden reports whether the named harness is hidden in the fleet config.
// It does not check that the name is a supported harness.
func IsHidden(p *paths.Paths, name string) (bool, error) {
	hidden, err := HiddenNames(p)
	if err != nil {
		return false, err
	}
	return slices.Contains(hidden, name), nil
}

// RequireManaged refuses a command that names a hidden harness, with the
// command that brings it back. Unknown names and unhidden harnesses pass,
// so callers keep their own unknown-name errors.
func RequireManaged(p *paths.Paths, name string) error {
	hidden, err := IsHidden(p, name)
	if err != nil {
		return err
	}
	if hidden {
		return fmt.Errorf("%s is hidden; run `fleet harness unhide %s` to manage it again", name, name)
	}
	return nil
}

// HiddenNames returns the harness names hidden in the fleet config, in the
// order they were hidden.
func HiddenNames(p *paths.Paths) ([]string, error) {
	cfg, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return nil, err
	}
	return cfg.HiddenHarnesses(), nil
}
