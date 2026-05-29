// Command frodo-ci is the CLI for the Frodo CI modular CI/CD framework.
package main

import (
	"fmt"
	"os"

	"github.com/frodo-ci/frodo-ci/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		// The root command silences cobra's own error printing so we control
		// the exact format and exit code here.
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
