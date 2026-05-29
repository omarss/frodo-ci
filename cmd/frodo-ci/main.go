// Command frodo-ci is the CLI for the Frodo CI modular CI/CD framework.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/omarss/frodo-ci/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		// The root command silences cobra's own error printing so we control
		// the exact format and exit code here. A quiet failure means the command
		// already printed its own diagnostics.
		if !errors.Is(err, cli.ErrExitQuiet) {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		os.Exit(1)
	}
}
