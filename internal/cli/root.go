// Package cli builds the suntimes command-line surface.
//
// The root command owns flag declaration, help and version output. Behaviour
// behind the flags (configuration loading, date-range resolution, sun-time
// calculation and table rendering) is added by later work and hangs off the
// Options struct below.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/philipf/suntimes/internal/buildinfo"
)

// Options holds the values parsed from the root command's flags. It is the
// single hand-off point between flag parsing and the packages that will
// consume these values.
type Options struct {
	// ConfigPath overrides the default ~/.config/suntimes/config.toml (SUN-2).
	ConfigPath string
	// Date selects a single day, as YYYY-MM-DD (SUN-14).
	Date string
	// Days is the number of days to show, starting today (SUN-12).
	Days int
	// From is the inclusive start of an explicit range, as YYYY-MM-DD (SUN-13).
	From string
	// To is the inclusive end of an explicit range, as YYYY-MM-DD (SUN-13).
	To string
}

const (
	shortDescription = "Print dawn, sunrise, sunset and dusk times"

	longDescription = `suntimes prints a table of daily light-related times — dawn, sunrise,
sunset and dusk — for a configured location and date range.

All values are computed locally from the latitude and longitude in your
configuration file; no network access is required.

With no date flags, suntimes shows the current date and the following six
days. Use --days, --from/--to or --date to choose a different range.`

	// placeholderNotice is printed until configuration loading and sun-time
	// calculation are implemented.
	placeholderNotice = "suntimes: nothing to show yet — configuration loading and sun-time calculation are not implemented."
)

// NewRootCommand builds the root `suntimes` command. A fresh command is
// returned on every call so that tests can execute it in isolation.
func NewRootCommand() *cobra.Command {
	opts := &Options{}

	cmd := &cobra.Command{
		Use:   "suntimes",
		Short: shortDescription,
		Long:  longDescription,
		// The command takes flags only; reject stray positional arguments.
		Args: cobra.NoArgs,
		// Errors are reported by main; cobra should not also print usage for
		// runtime failures (as opposed to flag/argument errors).
		SilenceUsage: true,
		Version:      buildinfo.Version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd, opts)
		},
	}

	cmd.SetVersionTemplate("{{.Name}} version {{.Version}}\n")

	flags := cmd.Flags()
	flags.StringVar(&opts.ConfigPath, "config", "",
		"path to the configuration file (default ~/.config/suntimes/config.toml)")
	flags.StringVar(&opts.Date, "date", "",
		"show a single date, as YYYY-MM-DD")
	flags.IntVar(&opts.Days, "days", 0,
		"number of days to show, starting today (default 7)")
	flags.StringVar(&opts.From, "from", "",
		"inclusive start of a date range, as YYYY-MM-DD")
	flags.StringVar(&opts.To, "to", "",
		"inclusive end of a date range, as YYYY-MM-DD")

	return cmd
}

// run executes the root command. It currently has nothing to compute, so it
// prints a placeholder and succeeds (SUN-30).
func run(cmd *cobra.Command, _ *Options) error {
	_, err := fmt.Fprintln(cmd.OutOrStdout(), placeholderNotice)
	return err
}
