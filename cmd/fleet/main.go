// Command fleet manages agent skills across AI coding agents. Paths derive
// from FLEET_HOME when set, otherwise the user's home directory.
package main

import (
	"fmt"
	"os"

	"github.com/zacong/fleet/internal/cli"
	"github.com/zacong/fleet/internal/paths"
)

func main() {
	p, err := paths.FromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fleet:", err)
		os.Exit(1)
	}
	if err := cli.NewRoot(p).Execute(); err != nil {
		os.Exit(1)
	}
}
