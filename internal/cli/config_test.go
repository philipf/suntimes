package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
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

// rowPattern is one plain result row: date, three-letter weekday, then four
// 24-hour times (SUN-21, SUN-22, SUN-23).
var rowPattern = regexp.MustCompile(
	`^(\d{4}-\d{2}-\d{2})  [A-Z][a-z]{2}(  (?:\d{2}:\d{2}|—)){4}$`)

// SUN-2, SUN-6, SUN-20: a valid file is read from the --config path, and its
// coordinates produce rows of times. SUN-11: the first row is today's.
func TestRunPrintsARowForTheConfiguredLocation(t *testing.T) {
	fakeHome(t)
	path := writeConfig(t, "latitude = -36.8485\nlongitude = 174.7633\ntimezone = \"Pacific/Auckland\"\n")

	// Bracket the run, because the local date can turn over mid-test.
	before := time.Now().In(time.Local).Format("2006-01-02")
	out, err := execute(t, "--config", path)
	after := time.Now().In(time.Local).Format("2006-01-02")
	if err != nil {
		t.Fatalf("run returned error %v, want nil", err)
	}

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 7 {
		t.Fatalf("run printed %d lines, want the default week of 7\n---\n%s", len(lines), out)
	}

	match := rowPattern.FindStringSubmatch(lines[0])
	if match == nil {
		t.Fatalf("output %q does not match a result row %s", lines[0], rowPattern)
	}
	// SUN-11: the window starts on the current date. Which days follow it is
	// covered deterministically, against a pinned clock, in dates_test.go.
	if date := match[1]; date != before && date != after {
		t.Errorf("first row is for %s, want today (%s or %s)", date, before, after)
	}
}

// SUN-8: with no timezone configured the run still succeeds, showing times in
// the host machine's local timezone. The configured name is issue #4.
func TestRunPrintsARowWithNoTimezoneConfigured(t *testing.T) {
	fakeHome(t)
	// 0,0 is the Gulf of Guinea, a valid location — not a missing value.
	path := writeConfig(t, "latitude = 0\nlongitude = 0\n")

	out, err := execute(t, "--config", path, "--days", "1")
	if err != nil {
		t.Fatalf("run returned error %v, want nil", err)
	}
	if !rowPattern.MatchString(strings.TrimSuffix(out, "\n")) {
		t.Errorf("output %q does not match a result row %s", out, rowPattern)
	}
}

// SUN-4, SUN-5: bad configuration ends the run with a descriptive error.
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
