package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

func newConfigCmd(p *paths.Paths) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage fleet's machine-local config",
		Long:  "Manage fleet's machine-local config at ~/.config/fleet/config.json (FLEET_HOME-aware). The file holds the pointer to the versioned skills repo.",
	}
	cmd.AddCommand(newConfigGetCmd(p))
	cmd.AddCommand(newConfigSetCmd(p))
	cmd.AddCommand(newConfigUnsetCmd(p))
	cmd.AddCommand(newConfigListCmd(p))
	return cmd
}

func normalizeConfigKey(k string) string {
	switch k {
	case "skills-repo", "skillsRepo":
		return "skills-repo"
	default:
		return ""
	}
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	return p
}

func newConfigGetCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print a config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if normalizeConfigKey(key) == "" {
				return fmt.Errorf("unknown config key %q (want skills-repo)", key)
			}
			repo, err := config.EffectiveRepo(p)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), repo)
			return err
		},
	}
}

func newConfigSetCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, val := args[0], args[1]
			if normalizeConfigKey(key) == "" {
				return fmt.Errorf("unknown config key %q (want skills-repo)", key)
			}
			expanded := expandPath(val)
			if !filepath.IsAbs(expanded) {
				return fmt.Errorf("skills repo path must be absolute: %q", val)
			}
			abs := filepath.Clean(expanded)
			st, err := os.Stat(abs)
			if err != nil {
				return fmt.Errorf("skills repo path does not exist: %q: %w", abs, err)
			}
			if !st.IsDir() {
				return fmt.Errorf("skills repo path is not a directory: %q", abs)
			}
			if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %q does not contain .git — not a git repo root\n", abs)
			}
			f, err := config.Load(p.FleetConfigFile())
			if err != nil {
				return err
			}
			f.SetSkillsRepo(abs)
			return config.Save(p.FleetConfigFile(), f)
		},
	}
}

func newConfigUnsetCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "unset <key>",
		Short: "Clear a config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if normalizeConfigKey(key) == "" {
				return fmt.Errorf("unknown config key %q (want skills-repo)", key)
			}
			f, err := config.Load(p.FleetConfigFile())
			if err != nil {
				return err
			}
			f.UnsetSkillsRepo()
			return config.Save(p.FleetConfigFile(), f)
		},
	}
}

func newConfigListCmd(p *paths.Paths) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List fleet config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			repo, err := config.EffectiveRepo(p)
			if err != nil {
				return err
			}
			if asJSON {
				obj := map[string]string{}
				if repo != "" {
					obj["skillsRepo"] = repo
				}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(obj)
			}
			if repo == "" {
				return nil
			}
			_, err = fmt.Fprintf(out, "skills-repo = %s\n", repo)
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON instead of a table")
	return cmd
}
