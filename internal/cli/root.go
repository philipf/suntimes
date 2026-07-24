// Package cli builds the suntimes command-line surface.
//
// The root command owns flag declaration, help and version output. Behaviour
// behind the flags (configuration loading, date-range resolution, sun-time
// calculation and table rendering) is added by later work and hangs off the
// Options struct below.
package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/philipf/suntimes/internal/buildinfo"
	"github.com/philipf/suntimes/internal/config"
	"github.com/philipf/suntimes/internal/render"
	"github.com/philipf/suntimes/internal/sun"
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
	// DaysSet records whether --days was supplied at all. The count alone
	// cannot say: an explicit `--days 0` and an absent flag are both zero, and
	// they mean opposite things — one is a mistake to report, the other the
	// default week (SUN-11, SUN-15).
	DaysSet bool
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
			// Only the parsed command knows which flags were actually typed.
			opts.DaysSet = cmd.Flags().Changed("days")

			err := run(cmd, opts)

			// SUN-15: flags that cannot be combined are a usage error, so the
			// usage that SilenceUsage suppresses for runtime failures is put
			// back for these — the same treatment cobra gives an unknown flag.
			var usage *usageError
			if errors.As(err, &usage) {
				cmd.SilenceUsage = false
			}
			return err
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

// run executes the root command: it resolves and loads the configuration,
// creating a sample on first run, then computes the sun times for the
// requested days and renders them.
func run(cmd *cobra.Command, opts *Options) error {
	path, err := config.Resolve(opts.ConfigPath)
	if err != nil {
		return err
	}

	created, err := config.CreateSampleIfMissing(path)
	if err != nil {
		return err
	}
	if created {
		// SUN-3: report the new file and stop; the sample's coordinates are an
		// example, so computing from them would be misleading.
		return report(cmd,
			fmt.Sprintf("Created a sample configuration at %s", path),
			"Edit it to set your location, then run suntimes again.")
	}

	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	// SUN-7, SUN-8: the configured timezone, already resolved and validated by
	// Load, or the host machine's local timezone when none is configured.
	// Everything downstream takes the display zone as a parameter, so nothing
	// else needs to know which it got.
	zone := cfg.Zone

	place := sun.Place{Latitude: cfg.Latitude, Longitude: cfg.Longitude}

	// SUN-11..SUN-17: the date flags become the days to show, ascending.
	dates, err := resolveDates(opts, currentDate(zone))
	if err != nil {
		return err
	}

	days := make([]sun.Day, 0, len(dates))
	for _, date := range dates {
		days = append(days, sun.Times(place, date))
	}

	return render.Table(cmd.OutOrStdout(), days, zone)
}

// report writes lines to the command's output stream, which tests replace with
// a buffer.
func report(cmd *cobra.Command, lines ...string) error {
	out := cmd.OutOrStdout()
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}
