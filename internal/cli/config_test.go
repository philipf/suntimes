package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	// The timezone tests below name real IANA zones, so the test binary carries
	// the database rather than depending on the host having one — the same
	// guarantee main gives the shipped binary.
	_ "time/tzdata"
)

// fakeHome points os.UserHomeDir at a temporary directory, so no test ever
// reads or writes the real ~/.config/suntimes.
func fakeHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)        // unix
	t.Setenv("USERPROFILE", home) // windows
	return home
}

// writeConfig puts contents in a fresh temporary file and returns its path.
func writeConfig(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}
	return path
}

// SUN-1, SUN-3: with no --config, the first run creates the sample at the
// default location and says where it put it.
func TestFirstRunCreatesSampleAtDefaultPath(t *testing.T) {
	home := fakeHome(t)
	want := filepath.Join(home, ".config", "suntimes", "config.toml")

	out, err := execute(t)
	if err != nil {
		t.Fatalf("first run returned error %v, want nil", err)
	}
	if _, statErr := os.Stat(want); statErr != nil {
		t.Fatalf("no sample configuration at the default path %s: %v", want, statErr)
	}
	if !strings.Contains(out, want) {
		t.Errorf("first-run output does not print the created path %q\n---\n%s", want, out)
	}
}

// SUN-2, SUN-3: --config decides where the sample is created.
func TestFirstRunCreatesSampleAtFlagPath(t *testing.T) {
	fakeHome(t)
	path := filepath.Join(t.TempDir(), "custom", "suntimes.toml")

	out, err := execute(t, "--config", path)
	if err != nil {
		t.Fatalf("first run returned error %v, want nil", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("no sample configuration at %s: %v", path, statErr)
	}
	if !strings.Contains(out, path) {
		t.Errorf("first-run output does not print the created path %q\n---\n%s", path, out)
	}

	// A second run finds the file and reports what it holds instead of
	// creating anything again.
	out, err = execute(t, "--config", path)
	if err != nil {
		t.Fatalf("second run returned error %v, want nil", err)
	}
	if strings.Contains(out, "Created") {
		t.Errorf("second run created a sample again:\n%s", out)
	}
}

// cellPattern is the shape of each column of a result row: a YYYY-MM-DD date, a
// three-letter weekday, then four 24-hour times or placeholders (SUN-21,
// SUN-22, SUN-23).
var cellPattern = []*regexp.Regexp{
	regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`),
	regexp.MustCompile(`^[A-Z][a-z]{2}$`),
	regexp.MustCompile(`^(?:\d{2}:\d{2}|—)$`),
	regexp.MustCompile(`^(?:\d{2}:\d{2}|—)$`),
	regexp.MustCompile(`^(?:\d{2}:\d{2}|—)$`),
	regexp.MustCompile(`^(?:\d{2}:\d{2}|—)$`),
}

// headings are the table's column names, in order (SUN-21).
var headings = []string{"Date", "Day", "Dawn", "Sunrise", "Sunset", "Dusk"}

// resultRows reads the command's output back as a table: it checks the borders
// and the header are there, then returns the cells of each data row.
//
// Nothing here strips escape codes. Every test runs with output going to a
// buffer, so a plain table is what the command must produce (SUN-26); a
// stripping parser would hide exactly the bug these tests exist to catch.
func resultRows(t *testing.T, out string) [][]string {
	t.Helper()

	if strings.ContainsRune(out, 0x1b) {
		t.Fatalf("output written to a buffer contains an escape code:\n%q", out)
	}

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) < 4 {
		t.Fatalf("output is %d lines, too few for a bordered table:\n%s", len(lines), out)
	}
	// Top border, header, header rule, at least one row, bottom border.
	for index, prefix := range map[int]string{0: "┌", 2: "├", len(lines) - 1: "└"} {
		if !strings.HasPrefix(lines[index], prefix) {
			t.Fatalf("line %d does not start with the border %q:\n%s", index, prefix, out)
		}
	}

	rows := make([][]string, 0, len(lines)-4)
	for index, line := range lines[1:] {
		if !strings.HasPrefix(line, "│") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "│"), "│")
		if len(cells) != len(headings) {
			t.Fatalf("line %d has %d columns, want %d:\n%s", index+1, len(cells), len(headings), out)
		}
		for column, cell := range cells {
			cells[column] = strings.TrimSpace(cell)
		}
		rows = append(rows, cells)
	}

	if len(rows) == 0 {
		t.Fatalf("output has no header row at all:\n%s", out)
	}
	if got := strings.Join(rows[0], "|"); got != strings.Join(headings, "|") {
		t.Fatalf("header row is %q, want %q", got, strings.Join(headings, "|"))
	}
	rows = rows[1:]
	if len(rows) == 0 {
		t.Fatalf("output has a header but no result rows:\n%s", out)
	}

	for index, row := range rows {
		for column, pattern := range cellPattern {
			if !pattern.MatchString(row[column]) {
				t.Fatalf("row %d column %d is %q, want %s\n---\n%s",
					index, column, row[column], pattern, out)
			}
		}
	}
	return rows
}

// SUN-2, SUN-6, SUN-20: a valid file is read from the --config path, and its
// coordinates produce rows of times. SUN-11: the first row is today's.
func TestRunPrintsARowForTheConfiguredLocation(t *testing.T) {
	fakeHome(t)
	const timezone = "Pacific/Auckland"
	path := writeConfig(t, "latitude = -36.8485\nlongitude = 174.7633\ntimezone = \""+timezone+"\"\n")

	// SUN-11's "today" is today in the configured timezone, not the host's, so
	// the expectation is read in that same zone. Bracketing the host zone
	// instead would pass only on a machine already set to it, and fail
	// anywhere the two are on different sides of midnight.
	zone, err := time.LoadLocation(timezone)
	if err != nil {
		t.Fatalf("loading %s returned error %v, want nil", timezone, err)
	}

	// Bracket the run, because the date in that zone can turn over mid-test.
	before := time.Now().In(zone).Format("2006-01-02")
	out, err := execute(t, "--config", path)
	after := time.Now().In(zone).Format("2006-01-02")
	if err != nil {
		t.Fatalf("run returned error %v, want nil", err)
	}

	rows := resultRows(t, out)
	if len(rows) != 7 {
		t.Fatalf("run printed %d rows, want the default week of 7\n---\n%s", len(rows), out)
	}

	// SUN-11: the window starts on the current date. Which days follow it is
	// covered deterministically, against a pinned clock, in dates_test.go.
	if date := rows[0][0]; date != before && date != after {
		t.Errorf("first row is for %s, want today (%s or %s)", date, before, after)
	}
}

// SUN-8: with no timezone configured the run still succeeds, showing times in
// the host machine's local timezone. Blank and absent mean the same thing, and
// neither is an error. That the fallback is time.Local itself, rather than
// merely something that renders, is pinned in internal/config, where the
// assertion does not have to depend on which zone the host actually has.
func TestRunPrintsARowWithNoTimezoneConfigured(t *testing.T) {
	// 0,0 is the Gulf of Guinea, a valid location — not a missing value.
	contents := map[string]string{
		"absent": "latitude = 0\nlongitude = 0\n",
		"blank":  "latitude = 0\nlongitude = 0\ntimezone = \"\"\n",
	}

	for name, config := range contents {
		t.Run(name, func(t *testing.T) {
			fakeHome(t)
			path := writeConfig(t, config)

			// --days 1 so the assertion stays about a single row; the
			// default range is a week.
			out, err := execute(t, "--config", path, "--days", "1")
			if err != nil {
				t.Fatalf("run returned error %v, want nil", err)
			}
			if rows := resultRows(t, out); len(rows) != 1 {
				t.Errorf("run printed %d rows, want 1\n---\n%s", len(rows), out)
			}
		})
	}
}

// SUN-7: the configured timezone decides the clock times printed. Two runs of
// the same location differ by exactly the offset between the zones configured,
// which no other setting could produce.
//
// Nothing here reads the host's local zone or the current time: the assertion
// is the difference between two runs, not the value of either. The zones are
// UTC and Kiritimati — the furthest east there is, at UTC+14 all year — so the
// expected difference is a constant that no daylight-saving rule can move.
func TestRunDisplaysTimesInTheConfiguredTimezone(t *testing.T) {
	const location = "latitude = 0\nlongitude = 0\n"

	fakeHome(t)
	atUTC := clockCells(t, singleRow(t, writeConfig(t, location+"timezone = \"UTC\"\n")))
	farEast := clockCells(t, singleRow(t, writeConfig(t, location+"timezone = \"Pacific/Kiritimati\"\n")))

	const minutesInDay = 24 * 60
	const offsetInMinutes = 14 * 60
	for column, want := range atUTC {
		// Modulo a day: at the equator the sun rises around 06:00 UTC, which is
		// already the evening before in Kiritimati.
		shifted := (want + offsetInMinutes) % minutesInDay

		// The two runs each ask for "today", and today in Kiritimati is usually
		// tomorrow in UTC — a different date, whose times differ by a few
		// seconds of the equation of time. That is enough to move a rounded
		// minute, so a minute of slack is expected; an hour of it is a bug.
		if difference := farEast[column] - shifted; difference < -1 || difference > 1 {
			t.Errorf("time column %d printed %s in Kiritimati and %s in UTC, want %s (UTC+14)",
				column, clock(farEast[column]), clock(want), clock(shifted))
		}
	}
}

// singleRow runs the command against a configuration file and returns the cells
// of the one row it printed.
func singleRow(t *testing.T, path string) []string {
	t.Helper()

	// --days 1: this helper's contract is one row, and the default is a week.
	out, err := execute(t, "--config", path, "--days", "1")
	if err != nil {
		t.Fatalf("run with %s returned error %v, want nil", path, err)
	}

	rows := resultRows(t, out)
	if len(rows) != 1 {
		t.Fatalf("run with %s printed %d rows, want 1\n---\n%s", path, len(rows), out)
	}
	return rows[0]
}

// clockCells reads the four time columns of a row as minutes since midnight.
func clockCells(t *testing.T, row []string) []int {
	t.Helper()

	const firstTimeColumn = 2 // date, day, then dawn, sunrise, sunset, dusk
	if len(row) != firstTimeColumn+4 {
		t.Fatalf("row %v has %d columns, want %d", row, len(row), firstTimeColumn+4)
	}

	minutes := make([]int, 0, 4)
	for _, cell := range row[firstTimeColumn:] {
		parsed, err := time.Parse("15:04", cell)
		if err != nil {
			t.Fatalf("unparsable time cell %q in row %v: %v", cell, row, err)
		}
		minutes = append(minutes, parsed.Hour()*60+parsed.Minute())
	}
	return minutes
}

// clock renders minutes since midnight as HH:MM, for failure messages.
func clock(minutes int) string {
	return time.Date(0, time.January, 1, 0, minutes, 0, 0, time.UTC).Format("15:04")
}

// SUN-4, SUN-5, SUN-9: bad configuration ends the run with a descriptive
// error.
func TestRunFailsOnInvalidConfiguration(t *testing.T) {
	tests := map[string]struct {
		contents string
		wants    []string
	}{
		"malformed toml":     {contents: "latitude = = 1\n", wants: []string{"TOML"}},
		"missing latitude":   {contents: "longitude = 10\n", wants: []string{"latitude", "missing"}},
		"missing longitude":  {contents: "latitude = 10\n", wants: []string{"longitude", "missing"}},
		"latitude too big":   {contents: "latitude = 91\nlongitude = 0\n", wants: []string{"latitude", "90"}},
		"longitude too big":  {contents: "latitude = 0\nlongitude = 181\n", wants: []string{"longitude", "180"}},
		"latitude too small": {contents: "latitude = -90.5\nlongitude = 0\n", wants: []string{"latitude", "-90"}},
		// SUN-9: an unrecognised zone stops the run rather than quietly
		// showing local time, which the user could not tell from the output.
		"unrecognised timezone": {
			contents: "latitude = 0\nlongitude = 0\ntimezone = \"Mars/Olympus_Mons\"\n",
			wants:    []string{"timezone", "Mars/Olympus_Mons", "IANA"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fakeHome(t)
			path := writeConfig(t, test.contents)

			out, err := execute(t, "--config", path)
			if err == nil {
				t.Fatalf("run succeeded with %s, want a non-zero exit\n---\n%s", name, out)
			}
			message := err.Error()
			if !strings.Contains(message, path) {
				t.Errorf("error %q does not name the configuration file %q", message, path)
			}
			for _, want := range test.wants {
				if !strings.Contains(message, want) {
					t.Errorf("error %q does not mention %q", message, want)
				}
			}
			if strings.Contains(out, "Usage:") {
				t.Errorf("a configuration error printed usage:\n%s", out)
			}
		})
	}
}
