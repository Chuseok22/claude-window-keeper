// Command limitping keeps Claude Code / Codex rate-limit windows back-to-back
// by pinging each provider the moment its 5h window resets.
package main

import (
	"fmt"
	"os"

	"github.com/Chuseok22/claude-window-keeper/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
