package harness

import (
	"fmt"
	"slices"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

// NotHidden returns every supported harness the user has not hidden, in
// All's order, whether or not it is installed. Commands that clean up
// fleet's own records (prune's state entries, doctor's stale state) use it,
// so an uninstalled harness's leftovers are still visible and removable.
// A config that cannot be read is an error, never a silent "nothing hidden".
func NotHidden(p *paths.Paths) ([]Adapter, error) {
	hidden, err := HiddenNames(p)
	if err != nil {
		return nil, err
	}
	var out []Adapter
	for _, a := range All(p) {
		if !slices.Contains(hidden, string(a.Harness())) {
			out = append(out, a)
		}
	}
	return out, nil
}

// Managed returns the adapters fleet acts on: the installed harnesses the
// user has not hidden, in All's order. Writes and projections use it.
// Installed stays detection only, and All stays the full list for listing.
func Managed(p *paths.Paths) ([]Adapter, error) {
	notHidden, err := NotHidden(p)
	if err != nil {
		return nil, err
	}
	var out []Adapter
	for _, a := range notHidden {
		if a.Installed() {
			out = append(out, a)
		}
	}
	return out, nil
}

// ByName returns the adapter for a supported harness name, and false when
// the name is not supported.
func ByName(adapters []Adapter, name string) (Adapter, bool) {
	for _, a := range adapters {
		if string(a.Harness()) == name {
			return a, true
		}
	}
	return nil, false
}

// RefuseHidden refuses a command that names a hidden harness, with the
// command that brings it back. Unknown names and unhidden harnesses pass,
// so callers keep their own unknown-name errors.
func RefuseHidden(p *paths.Paths, name string) error {
	hidden, err := HiddenNames(p)
	if err != nil {
		return err
	}
	if slices.Contains(hidden, name) {
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
