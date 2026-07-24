package cli

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/philipf/suntimes/internal/buildinfo"
	"github.com/philipf/suntimes/internal/render"
)

// TestMain fixes the styling decision before any test in this package runs.
//
// Every test captures output in a buffer, so plain is what they must all see.
// The production default is detection against the process's real standard
// output, and `go test` sometimes hands the test binary the developer's own
// terminal — which would make the suite pass or fail depending on how it was
// launched. Pinning it here keeps the question "does the command style what it
// writes" rather than "where was this run from".
func TestMain(m *testing.M) {
	outputStyle = func() render.Style { return render.Plain() }
	os.Exit(m.Run())
}

// pinStyle replaces the styling decision for the duration of one test, so both
// sides of it can be exercised without a terminal to run in.
func pinStyle(t *testing.T, style render.Style) {
	t.Helper()

	previous := outputStyle
	outputStyle = func() render.Style { return style }
	t.Cleanup(func() { outputStyle = previous })
}

// execute runs a fresh root command with the given args, capturing everything
// it writes. It returns the combined output and the error Execute reported.
func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return out.String(), err
}

// stubVersion replaces the linker-stamped version for the duration of a test.
func stubVersion(t *testing.T, version string) {
	t.Helper()

	previous := buildinfo.Version
	buildinfo.Version = version
	t.Cleanup(func() { buildinfo.Version = previous })
}

// SUN-29: --version prints the application version and exits zero.
func TestVersionFlagPrintsStampedVersion(t *testing.T) {
	stubVersion(t, "v1.2.3-test")

	out, err := execute(t, "--version")
	if err != nil {
		t.Fatalf("--version returned error %v, want nil (exit status zero)", err)
	}
	if !strings.Contains(out, "v1.2.3-test") {
		t.Errorf("--version output %q does not contain the stamped version", out)
	}
	if !strings.Contains(out, "suntimes") {
		t.Errorf("--version output %q does not name the command", out)
	}
}

// The default version is a usable placeholder for unstamped local builds.
func TestDefaultVersionIsDev(t *testing.T) {
	if buildinfo.Version != "dev" {
		t.Errorf("buildinfo.Version = %q, want %q for an unstamped build", buildinfo.Version, "dev")
	}
}

// SUN-28: --help and -h print usage describing all flags and exit zero.
func TestHelpFlagsDescribeAllFlags(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			out, err := execute(t, flag)
			if err != nil {
				t.Fatalf("%s returned error %v, want nil (exit status zero)", flag, err)
			}

			for _, want := range []string{
				"suntimes",
				"Usage:",
				"Flags:",
				"--config",
				"--date",
				"--days",
				"--from",
				"--to",
				"--help",
				"--version",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("%s output does not mention %q\n---\n%s", flag, want, out)
				}
			}
		})
	}
}

// SUN-30: a successful run exits zero.
func TestRunWithNoArgsSucceeds(t *testing.T) {
	fakeHome(t)

	out, err := execute(t)
	if err != nil {
		t.Fatalf("bare run returned error %v, want nil (exit status zero)", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Error("bare run printed nothing, want a report of what it did")
	}
}

// Flags parse into Options so later work has a single hand-off point.
func TestFlagsPopulateOptions(t *testing.T) {
	var captured *Options

	cmd := NewRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		captured = optionsFromFlags(t, c)
		return nil
	}
	cmd.SetArgs([]string{
		"--config", "/tmp/config.toml",
		"--date", "2026-07-25",
		"--days", "3",
		"--from", "2026-07-01",
		"--to", "2026-07-07",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error %v, want nil", err)
	}

	want := &Options{
		ConfigPath: "/tmp/config.toml",
		Date:       "2026-07-25",
		Days:       3,
		From:       "2026-07-01",
		To:         "2026-07-07",
	}
	if *captured != *want {
		t.Errorf("parsed options = %+v, want %+v", *captured, *want)
	}
}

// optionsFromFlags reads the parsed flag values back off a command, which is
// how a test observes what NewRootCommand bound without exporting internals.
func optionsFromFlags(t *testing.T, cmd *cobra.Command) *Options {
	t.Helper()

	str := func(name string) string {
		value, err := cmd.Flags().GetString(name)
		if err != nil {
			t.Fatalf("reading flag %q: %v", name, err)
		}
		return value
	}
	days, err := cmd.Flags().GetInt("days")
	if err != nil {
		t.Fatalf("reading flag %q: %v", "days", err)
	}

	return &Options{
		ConfigPath: str("config"),
		Date:       str("date"),
		Days:       days,
		From:       str("from"),
		To:         str("to"),
	}
}

// Unknown flags are a usage error, not a silent success.
func TestUnknownFlagIsAnError(t *testing.T) {
	if _, err := execute(t, "--nope"); err == nil {
		t.Fatal("unknown flag returned nil error, want a non-zero exit")
	}
}

// Positional arguments are rejected; the command is flag-driven.
func TestPositionalArgumentIsAnError(t *testing.T) {
	if _, err := execute(t, "tomorrow"); err == nil {
		t.Fatal("positional argument returned nil error, want a non-zero exit")
	}
}

// reverseVideo is the escape the theme highlights today's row with (SUN-25).
const reverseVideo = "\x1b[7m"

// SUN-26: when the styling decision says plain — which is what redirected
// output resolves to — not one escape byte reaches the stream, so the table
// stays greppable and diffable. This is asserted on the bytes rather than on
// how the output looks.
func TestRunWritesPlainTextWhenOutputIsNotStyled(t *testing.T) {
	fakeHome(t)
	pinToday(t, "2026-07-25")
	pinStyle(t, render.Plain())
	path := writeConfig(t, "latitude = -36.8485\nlongitude = 174.7633\n")

	out, err := execute(t, "--config", path, "--days", "3")
	if err != nil {
		t.Fatalf("run returned error %v, want nil\n---\n%s", err, out)
	}

	if index := strings.IndexByte(out, 0x1b); index >= 0 {
		t.Fatalf("escape byte at offset %d of plain output:\n%q", index, out)
	}
	// The table itself is still there — a run that printed nothing would also
	// have no escapes in it.
	if rows := resultRows(t, out); len(rows) != 3 {
		t.Errorf("run printed %d rows, want 3\n---\n%s", len(rows), out)
	}
}

// SUN-25, SUN-27a: when the styling decision says style it, the table is themed
// and today's row — the same date the range was built around — is the one
// picked out.
func TestRunHighlightsTodayWhenOutputIsStyled(t *testing.T) {
	fakeHome(t)
	today := pinToday(t, "2026-07-25")
	pinStyle(t, render.Themed())
	path := writeConfig(t, "latitude = -36.8485\nlongitude = 174.7633\n")

	out, err := execute(t, "--config", path, "--from", "2026-07-24", "--to", "2026-07-26")
	if err != nil {
		t.Fatalf("run returned error %v, want nil\n---\n%s", err, out)
	}
	if !strings.Contains(out, "\x1b") {
		t.Fatalf("styled output carries no escape codes at all:\n%q", out)
	}

	// Today is the middle row, so a renderer that highlighted the first row on
	// principle would fail here.
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		isToday := strings.Contains(line, today.String())
		if got := strings.Contains(line, reverseVideo); got != isToday {
			t.Errorf("line highlighted = %t, want %t:\n%q", got, isToday, line)
		}
	}
}

// SUN-25: a range that does not contain today highlights nothing. 29 February
// 2028 is a date the pinned clock will never be, and the run must neither
// panic nor pick a row to highlight for want of the right one.
func TestRunHighlightsNothingWhenTodayIsOutsideTheRange(t *testing.T) {
	fakeHome(t)
	pinToday(t, "2026-07-25")
	pinStyle(t, render.Themed())
	path := writeConfig(t, "latitude = -36.8485\nlongitude = 174.7633\n")

	out, err := execute(t, "--config", path, "--date", "2028-02-29")
	if err != nil {
		t.Fatalf("run returned error %v, want nil\n---\n%s", err, out)
	}
	if strings.Contains(out, reverseVideo) {
		t.Errorf("a row is highlighted although today is outside the range:\n%q", out)
	}
	if !strings.Contains(out, "\x1b") {
		t.Errorf("styled output carries no escape codes at all:\n%q", out)
	}
}

// longyearbyenConfig is Svalbard at 78.22°N, in UTC so the printed clock times
// do not depend on the host's timezone database resolving Arctic/Longyearbyen.
// Which events occur is a property of the calendar date and the latitude, so
// the zone cannot change the pattern of placeholders either way.
const longyearbyenConfig = "latitude = 78.22\nlongitude = 15.63\ntimezone = \"UTC\"\n"

// SUN-27, end to end: a real polar location run through the whole command
// prints a placeholder where an event does not occur, and a time where one
// does — in the same row, for the dates where the two thresholds disagree.
//
// The two mixed ranges run opposite ways round. In November the sun climbs into
// civil twilight but not to the horizon, so dawn and dusk are real and sunrise
// and sunset are not; in April it clears the horizon but never falls 6° below
// it again, so the reverse. A run that could only blank a whole row would fail
// both.
func TestRunShowsPlaceholdersForPolarNoEventDays(t *testing.T) {
	fakeHome(t)
	pinToday(t, "2026-07-25")
	pinStyle(t, render.Plain())
	path := writeConfig(t, longyearbyenConfig)

	tests := []struct {
		name string
		from string
		to   string
		// want is each row's four time columns, with "—" for a placeholder and
		// "time" for anything that must be a clock reading.
		want [][4]string
	}{
		{
			name: "polar night with civil twilight",
			from: "2026-11-03", to: "2026-11-05",
			want: [][4]string{
				{"time", "—", "—", "time"},
				{"time", "—", "—", "time"},
				{"time", "—", "—", "time"},
			},
		},
		{
			name: "continuous civil twilight",
			from: "2026-04-08", to: "2026-04-10",
			want: [][4]string{
				{"—", "time", "time", "—"},
				{"—", "time", "time", "—"},
				{"—", "time", "time", "—"},
			},
		},
		{
			name: "midnight sun",
			from: "2026-06-20", to: "2026-06-21",
			want: [][4]string{
				{"—", "—", "—", "—"},
				{"—", "—", "—", "—"},
			},
		},
		{
			name: "an ordinary polar day, unaffected",
			from: "2026-03-01", to: "2026-03-01",
			want: [][4]string{
				{"time", "time", "time", "time"},
			},
		},
	}

	clock := regexp.MustCompile(`^\d{2}:\d{2}$`)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, err := execute(t, "--config", path, "--from", test.from, "--to", test.to)
			if err != nil {
				t.Fatalf("run returned error %v, want nil\n---\n%s", err, out)
			}

			rows := resultRows(t, out)
			if len(rows) != len(test.want) {
				t.Fatalf("run printed %d rows, want %d\n---\n%s", len(rows), len(test.want), out)
			}

			for index, want := range test.want {
				// Columns 2..5 are dawn, sunrise, sunset and dusk (SUN-21).
				for column, expected := range want {
					got := rows[index][column+2]
					switch expected {
					case render.Placeholder:
						if got != render.Placeholder {
							t.Errorf("row %d column %d = %q, want the placeholder %q\n---\n%s",
								index, column+2, got, render.Placeholder, out)
						}
					default:
						if !clock.MatchString(got) {
							t.Errorf("row %d column %d = %q, want a HH:MM time\n---\n%s",
								index, column+2, got, out)
						}
					}
				}
			}
		})
	}
}

// SUN-5, SUN-27: a NaN coordinate is rejected before it can reach the
// astronomy. `latitude = nan` is valid TOML, and NaN compares false against
// every bound, so nothing but an explicit check stops it — and go-sunrise turns
// a NaN latitude into a time in the year 292277026596 rather than into an
// absent event, which is exactly the misleading value SUN-27 exists to keep out
// of the table.
func TestRunRejectsCoordinatesThatAreNotNumbers(t *testing.T) {
	fakeHome(t)
	pinStyle(t, render.Plain())

	for name, contents := range map[string]string{
		"NaN latitude":       "latitude = nan\nlongitude = 15.63\n",
		"NaN longitude":      "latitude = 78.22\nlongitude = nan\n",
		"infinite latitude":  "latitude = inf\nlongitude = 15.63\n",
		"infinite longitude": "latitude = 78.22\nlongitude = -inf\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, contents)

			out, err := execute(t, "--config", path, "--date", "2026-06-21")
			if err == nil {
				t.Fatalf("run accepted %s, want a non-zero exit\n---\n%s", name, out)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error %q does not name the file %q", err, path)
			}
			if strings.Contains(out, "│") {
				t.Errorf("a table was printed for %s:\n%s", name, out)
			}
		})
	}
}
