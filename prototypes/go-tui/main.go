// Prototype C: skillctl in Go — Cobra CLI + Bubble Tea v2 TUI.
// Dummy CLI over an embedded fixture; nothing writes to disk.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed skills.json
var fixtureJSON []byte

type root struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type skill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Enabled     bool     `json:"enabled"`
	Roots       []string `json:"roots"`
}

type fixture struct {
	Version int     `json:"version"`
	Roots   []root  `json:"roots"`
	Skills  []skill `json:"skills"`
}

func loadFixture() fixture {
	var f fixture
	if err := json.Unmarshal(fixtureJSON, &f); err != nil {
		fmt.Fprintf(os.Stderr, "skillctl: bad embedded fixture: %v\n", err)
		os.Exit(1)
	}
	return f
}

// expandRows duplicates fixture rows with suffixed names up to n entries.
func expandRows(n int) []skill {
	f := loadFixture()
	if n <= 0 || n <= len(f.Skills) {
		return f.Skills
	}
	rows := make([]skill, 0, n)
	rows = append(rows, f.Skills...)
	for i := len(f.Skills); i < n; i++ {
		base := f.Skills[i%len(f.Skills)]
		dup := base
		dup.Name = fmt.Sprintf("%s-%d", base.Name, i/len(f.Skills)+1)
		rows = append(rows, dup)
	}
	return rows
}

func lookupSkills(names []string) ([]skill, error) {
	f := loadFixture()
	byName := make(map[string]skill, len(f.Skills))
	for _, s := range f.Skills {
		byName[s.Name] = s
	}
	out := make([]skill, 0, len(names))
	for _, n := range names {
		s, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("unknown skill: %s", n)
		}
		out = append(out, s)
	}
	return out, nil
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "skillctl",
		Short: "prototype skill activation manager (dummy CLI, fixture data only)",
		Long:  "skillctl is a prototype skill activation manager. All data comes from an embedded fixture; nothing touches the filesystem.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}

	// skillctl list [--json] [--rows N]
	var listJSON bool
	var listRows int
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "print the fixture as a table",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			f := loadFixture()
			if listJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(f)
			}
			printTable(expandRows(listRows))
			return nil
		},
	}
	listCmd.Flags().BoolVar(&listJSON, "json", false, "print the fixture as JSON")
	listCmd.Flags().IntVar(&listRows, "rows", len(loadFixture().Skills), "stress fixture: duplicate rows up to N entries")
	rootCmd.AddCommand(listCmd)

	// skillctl enable <skill>...
	enableCmd := &cobra.Command{
		Use:   "enable <skill>...",
		Short: "print \"would enable <skill>\" per name",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skills, err := lookupSkills(args)
			if err != nil {
				return err
			}
			for _, s := range skills {
				fmt.Printf("would enable %s\n", s.Name)
			}
			return nil
		},
	}
	rootCmd.AddCommand(enableCmd)

	// skillctl disable <skill>...
	disableCmd := &cobra.Command{
		Use:   "disable <skill>...",
		Short: "print \"would disable <skill>\" per name",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skills, err := lookupSkills(args)
			if err != nil {
				return err
			}
			for _, s := range skills {
				fmt.Printf("would disable %s\n", s.Name)
			}
			return nil
		},
	}
	rootCmd.AddCommand(disableCmd)

	// skillctl toggle <skill>...
	toggleCmd := &cobra.Command{
		Use:   "toggle <skill>...",
		Short: "print the resolved action per name based on fixture state",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skills, err := lookupSkills(args)
			if err != nil {
				return err
			}
			for _, s := range skills {
				if s.Enabled {
					fmt.Printf("would disable %s (currently enabled)\n", s.Name)
				} else {
					fmt.Printf("would enable %s (currently disabled)\n", s.Name)
				}
			}
			return nil
		},
	}
	rootCmd.AddCommand(toggleCmd)

	// skillctl root list
	rootGroup := &cobra.Command{
		Use:   "root",
		Short: "skill root directories",
		Args:  cobra.NoArgs,
	}
	rootGroup.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "print the fixture's root names and paths",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			f := loadFixture()
			for _, r := range f.Roots {
				fmt.Printf("%-12s %s\n", r.Name, r.Path)
			}
			return nil
		},
	})
	rootCmd.AddCommand(rootGroup)

	// skillctl doctor
	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "print canned findings from the fixture",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			f := loadFixture()
			enabled, disabled := 0, 0
			multi := []string{}
			known := map[string]bool{}
			for _, r := range f.Roots {
				known[r.Name] = true
			}
			for _, s := range f.Skills {
				if s.Enabled {
					enabled++
				} else {
					disabled++
				}
				if len(s.Roots) > 1 {
					multi = append(multi, s.Name)
				}
				for _, rn := range s.Roots {
					if !known[rn] {
						fmt.Printf("warn  unknown root %q referenced by %s\n", rn, s.Name)
					}
				}
			}
			fmt.Printf("ok    fixture loads: %d skills, %d roots, all root references resolve\n", len(f.Skills), len(f.Roots))
			fmt.Printf("warn  %d skills are disabled (%d enabled)\n", disabled, enabled)
			fmt.Printf("info  %d skills live in multiple roots: %s\n", len(multi), strings.Join(multi, ", "))
			return nil
		},
	}
	rootCmd.AddCommand(doctorCmd)

	// skillctl tui [--rows N] [--bench]
	var tuiRows int
	var tuiBench bool
	tuiCmd := &cobra.Command{
		Use:   "tui",
		Short: "open the interactive manager",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTui(expandRows(tuiRows), tuiBench)
		},
	}
	tuiCmd.Flags().IntVar(&tuiRows, "rows", len(loadFixture().Skills), "stress fixture: duplicate rows up to N entries")
	tuiCmd.Flags().BoolVar(&tuiBench, "bench", false, "print startup→first-frame milliseconds to stderr, then exit")
	rootCmd.AddCommand(tuiCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// printTable renders the same row shape as the TUI, non-interactive, at a
// fixed 100-column width.
func printTable(rows []skill) {
	const width = 100
	nameW := 8
	for _, s := range rows {
		if n := len([]rune(s.Name)); n > nameW && n <= 30 {
			nameW = n
		}
	}
	for _, s := range rows {
		fmt.Println(renderRow(s, width, nameW, false, false))
	}
}
