// Command suntimes prints dawn, sunrise, sunset and dusk for a configured
// location and date range, computed offline.
package main

import (
	"os"

	// Embed the IANA timezone database in the binary so a configured timezone
	// resolves even where the host has no database of its own — a scratch
	// container, or Windows, which has none in the form Go reads (NFR-1,
	// SUN-7). Go consults the host's database first and only falls back to this
	// copy, so a machine with fresher data still wins.
	_ "time/tzdata"

	"github.com/philipf/suntimes/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		// Cobra has already written the error and usage hint to stderr.
		os.Exit(1)
	}
}
