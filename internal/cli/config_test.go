package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// SUN-2: a valid file is read from the --config path and its values reported.
func TestRunReportsResolvedConfiguration(t *testing.T) {
	fakeHome(t)
	path := writeConfig(t, "latitude = -36.8485\nlongitude = 174.7633\ntimezone = \"Pacific/Auckland\"\n")

	out, err := execute(t, "--config", path)
	if err != nil {
		t.Fatalf("run returned error %v, want nil", err)
	}

	for _, want := range []string{path, "-36.8485", "174.7633", "Pacific/Auckland"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not report %q\n---\n%s", want, out)
		}
	}
}

// SUN-8: no configured timezone is reported as the machine's local one.
func TestRunReportsSystemTimezoneWhenUnset(t *testing.T) {
	fakeHome(t)
	path := writeConfig(t, "latitude = 0\nlongitude = 0\n")

	out, err := execute(t, "--config", path)
	if err != nil {
		t.Fatalf("run returned error %v, want nil", err)
	}
	if !strings.Contains(out, systemTimezoneNotice) {
		t.Errorf("output does not report the fallback timezone\n---\n%s", out)
	}
	// 0,0 is the Gulf of Guinea, a valid location — not a missing value.
	if !strings.Contains(out, "Latitude:      0") {
		t.Errorf("output does not report latitude 0\n---\n%s", out)
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
