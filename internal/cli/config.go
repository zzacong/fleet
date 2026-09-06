package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

const configKeyAdoptTarget = "adopt-target"

var allowedConfigKeys = []string{configKeyAdoptTarget}

func newConfigCmd(p *paths.Paths) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage fleet's machine-local config",
		Long: "Manage fleet's machine-local config at ~/.config/fleet/config.json (FLEET_HOME-aware). The file holds the tracked customs repos (the explicit repo-root list, managed by `fleet skill pull`) and the adopt-target collection dir.\n\n" +
			"Available keys:\n" +
			"  adopt-target  Absolute path to the adopt-target collection dir (where `fleet skill adopt` lands). Alias: adoptTarget. Unset means the fleet-home fallback. No env override.",
	}
	cmd.AddCommand(newConfigGetCmd(p))
	cmd.AddCommand(newConfigSetCmd(p))
	cmd.AddCommand(newConfigUnsetCmd(p))
	cmd.AddCommand(newConfigListCmd(p))
	return cmd
}

func normalizeConfigKey(k string) string {
	switch k {
	case "adopt-target", "adoptTarget":
		return "adopt-target"
	default:
		return ""
	}
}

// unknownKeyError fails an unknown (or retired) config key with the
// multi-repo story: the tracked list plus the adopt target. The retired
// single-pointer key names its replacement instead of reading anything.
func unknownKeyError(key string) error {
	switch key {
	case "skills-repo", "skillsRepo":
		return fmt.Errorf("unknown config key %q (the single repo pointer is retired; tracked repos live in the repo list managed by `fleet skill pull`, and the adopt default in `adopt-target`)", key)
	default:
		return fmt.Errorf("unknown config key %q (want adopt-target)", key)
	}
}

func expandPath(p string) string {
	return config.ExpandPath(p)
}

func newConfigGetCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:       "get <key>",
		Short:     "Print a config value",
		Long:      "Print the value for a config key (adopt-target has no env override). Keys:\n\n  adopt-target  Absolute path to the adopt-target collection dir. Alias: adoptTarget.\n\nPrints empty and exits 0 when unset; unknown keys fail. The tracked repo list is shown by `fleet config list` and managed by `fleet skill pull`.",
		Example:   "  fleet config get adopt-target",
		ValidArgs: allowedConfigKeys,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return allowedConfigKeys, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			switch normalizeConfigKey(key) {
			case configKeyAdoptTarget:
				f, err := config.Load(p.FleetConfigFile())
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), f.AdoptTarget())
				return err
			default:
				return unknownKeyError(key)
			}
		},
	}
}

func newConfigSetCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:       "set <key> <value>",
		Short:     "Set a config value",
		Long:      "Set a config value. Keys:\n\n  adopt-target  Absolute path to the adopt-target collection dir (where adopts land). ~/ expands to the home directory. Alias: adoptTarget. The directory is created on demand by adopt; set does not require it to exist.\n\nValidates the path is absolute. Writes atomically and preserves unknown fields. The tracked repo list is managed by `fleet skill pull`, not by set.",
		Example:   "  fleet config set adopt-target ~/Developer/customs/skills",
		ValidArgs: allowedConfigKeys,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return allowedConfigKeys, cobra.ShellCompDirectiveNoFileComp
			}
			if len(args) == 1 {
				return nil, cobra.ShellCompDirectiveFilterDirs
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, val := args[0], args[1]
			switch normalizeConfigKey(key) {
			case configKeyAdoptTarget:
				return setAdoptTarget(p, val)
			default:
				return unknownKeyError(key)
			}
		},
	}
}

// setAdoptTarget validates the collection dir (absolute after home
// expansion; existence is not required — adopt creates it on demand) and
// saves it, preserving unknown fields.
func setAdoptTarget(p *paths.Paths, val string) error {
	abs, err := config.AbsolutePath(val)
	if err != nil {
		return err
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return err
	}
	f.SetAdoptTarget(abs)
	return config.Save(p.FleetConfigFile(), f)
}

func newConfigUnsetCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:       "unset <key>",
		Short:     "Clear a config value",
		Long:      "Clear a config value. Keys:\n\n  adopt-target  Absolute path to the adopt-target collection dir. Alias: adoptTarget.\n\nWrites atomically and preserves unknown fields.",
		Example:   "  fleet config unset adopt-target",
		ValidArgs: allowedConfigKeys,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return allowedConfigKeys, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			f, err := config.Load(p.FleetConfigFile())
			if err != nil {
				return err
			}
			switch normalizeConfigKey(key) {
			case configKeyAdoptTarget:
				f.UnsetAdoptTarget()
			default:
				return unknownKeyError(key)
			}
			return config.Save(p.FleetConfigFile(), f)
		},
	}
}

func newConfigListCmd(p *paths.Paths) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List fleet config",
		Long:  "List fleet's machine-local config: the tracked repo-root list in precedence order (managed by `fleet skill pull`) and the adopt-target collection dir (unset means the fleet-home fallback).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			f, err := config.Load(p.FleetConfigFile())
			if err != nil {
				return err
			}
			repos := f.SkillsRepos()
			target := f.AdoptTarget()
			if asJSON {
				obj := map[string]any{}
				if len(repos) > 0 {
					obj["skillsRepos"] = repos
				}
				if target != "" {
					obj["adoptTarget"] = target
				}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(obj)
			}
			for _, repo := range repos {
				if _, err := fmt.Fprintf(out, "skills-repos = %s\n", repo); err != nil {
					return err
				}
			}
			if target != "" {
				_, err = fmt.Fprintf(out, "adopt-target = %s\n", target)
				return err
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON instead of a table")
	return cmd
}
