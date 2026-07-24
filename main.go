// Command suntimes prints dawn, sunrise, sunset and dusk for a configured
// location and date range, computed offline.
package main

import (
	"os"

	"github.com/philipf/suntimes/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		// Cobra has already written the error and usage hint to stderr.
		os.Exit(1)
	}
}
