package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig puts contents in a fresh temporary file and returns its path.
// Nothing here ever touches the real ~/.config/suntimes.
func writeConfig(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}
	return path
}

// fakeHome points os.UserHomeDir at a temporary directory.
func fakeHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)        // unix
	t.Setenv("USERPROFILE", home) // windows
	return home
}

// SUN-1: the default location is ~/.config/suntimes/config.toml.
func TestDefaultPathIsUnderHomeConfig(t *testing.T) {
	home := fakeHome(t)

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath returned error %v, want nil", err)
	}

	want := filepath.Join(home, ".config", "suntimes", "config.toml")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

// SUN-1: with no --config, Resolve falls back to the default location.
func TestResolveWithoutFlagUsesDefaultPath(t *testing.T) {
	fakeHome(t)

	want, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath returned error %v, want nil", err)
	}

	got, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve returned error %v, want nil", err)
	}
	if got != want {
		t.Errorf("Resolve(\"\") = %q, want the default path %q", got, want)
	}
}

// SUN-2: --config wins over the default location.
func TestResolveWithFlagUsesThatPath(t *testing.T) {
	fakeHome(t)

	want := filepath.Join(t.TempDir(), "elsewhere.toml")
	got, err := Resolve(want)
	if err != nil {
		t.Fatalf("Resolve returned error %v, want nil", err)
	}
	if got != want {
		t.Errorf("Resolve(%q) = %q, want the flag path", want, got)
	}
}

// SUN-3: a missing file becomes a sample on disk, parent directories and all,
// and that sample is itself loadable configuration.
func TestCreateSampleIfMissingWritesLoadableSample(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "config.toml")

	created, err := CreateSampleIfMissing(path)
	if err != nil {
		t.Fatalf("CreateSampleIfMissing returned error %v, want nil", err)
	}
	if !created {
		t.Fatal("CreateSampleIfMissing reported no creation, want true for a missing file")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("sample was not written to %s: %v", path, err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("the written sample does not load: %v", err)
	}
	if cfg.Latitude < minLatitude || cfg.Latitude > maxLatitude {
		t.Errorf("sample latitude %g is out of range", cfg.Latitude)
	}
	if cfg.Longitude < minLongitude || cfg.Longitude > maxLongitude {
		t.Errorf("sample longitude %g is out of range", cfg.Longitude)
	}
	if cfg.Timezone == "" {
		t.Error("sample timezone is empty, want the example IANA name to survive the round trip")
	}
}

// The sample documents every key and its valid range in comments.
func TestSampleFileIsCommentedAndDocumentsRanges(t *testing.T) {
	for _, want := range []string{
		"latitude", "longitude", "timezone",
		"-90", "90", "-180", "180",
		"IANA",
	} {
		if !strings.Contains(SampleFile, want) {
			t.Errorf("sample configuration does not mention %q", want)
		}
	}

	comments := 0
	for _, line := range strings.Split(SampleFile, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			comments++
		}
	}
	if comments < 3 {
		t.Errorf("sample configuration has %d comment lines, want a comment for each key", comments)
	}
}

// An existing file is never overwritten by the first-run path.
func TestCreateSampleIfMissingLeavesExistingFileAlone(t *testing.T) {
	const existing = "latitude = 1.0\nlongitude = 2.0\n"
	path := writeConfig(t, existing)

	created, err := CreateSampleIfMissing(path)
	if err != nil {
		t.Fatalf("CreateSampleIfMissing returned error %v, want nil", err)
	}
	if created {
		t.Error("CreateSampleIfMissing reported a creation for a file that already exists")
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config back: %v", err)
	}
	if string(contents) != existing {
		t.Errorf("existing configuration was overwritten:\n%s", contents)
	}
}

// SUN-4: unparseable TOML is an error that names the file.
func TestLoadRejectsMalformedTOML(t *testing.T) {
	path := writeConfig(t, "latitude = = 12\nthis is not toml\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted malformed TOML, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the file %q", err, path)
	}
	if !strings.Contains(err.Error(), "TOML") {
		t.Errorf("error %q does not say the file is not valid TOML", err)
	}
}

// A file that is not there at all is an error; creating it is the caller's job.
func TestLoadReportsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.toml")

	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted a missing file, want an error")
	}
}

// SUN-5: each required coordinate must be present.
func TestLoadRejectsMissingCoordinates(t *testing.T) {
	tests := map[string]struct {
		contents string
		missing  string
	}{
		"no latitude":  {contents: "longitude = 174.7633\n", missing: "latitude"},
		"no longitude": {contents: "latitude = -36.8485\n", missing: "longitude"},
		"empty file":   {contents: "# nothing set\n", missing: "latitude"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, test.contents)

			_, err := Load(path)
			if err == nil {
				t.Fatalf("Load accepted a config with no %s, want an error", test.missing)
			}
			if !strings.Contains(err.Error(), test.missing) {
				t.Errorf("error %q does not name the missing key %q", err, test.missing)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error %q does not name the file %q", err, path)
			}
		})
	}
}

// SUN-5: the bounds are inclusive, and zero is a real coordinate.
func TestLoadAcceptsValuesInRange(t *testing.T) {
	tests := map[string]struct {
		contents  string
		latitude  float64
		longitude float64
	}{
		"gulf of guinea": {contents: "latitude = 0.0\nlongitude = 0.0\n"},
		"integer zero":   {contents: "latitude = 0\nlongitude = 0\n"},
		"south pole":     {contents: "latitude = -90\nlongitude = -180\n", latitude: -90, longitude: -180},
		"north pole":     {contents: "latitude = 90\nlongitude = 180\n", latitude: 90, longitude: 180},
		"auckland": {
			contents:  "latitude = -36.8485\nlongitude = 174.7633\n",
			latitude:  -36.8485,
			longitude: 174.7633,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, test.contents)

			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load returned error %v, want nil", err)
			}
			if cfg.Latitude != test.latitude {
				t.Errorf("latitude = %g, want %g", cfg.Latitude, test.latitude)
			}
			if cfg.Longitude != test.longitude {
				t.Errorf("longitude = %g, want %g", cfg.Longitude, test.longitude)
			}
			if cfg.Path != path {
				t.Errorf("Path = %q, want the file it was read from %q", cfg.Path, path)
			}
		})
	}
}

// SUN-5: just outside the bounds is rejected, as is a non-numeric value.
func TestLoadRejectsOutOfRangeCoordinates(t *testing.T) {
	tests := map[string]struct {
		contents string
		key      string
	}{
		"latitude just above 90":     {contents: "latitude = 90.0001\nlongitude = 0\n", key: "latitude"},
		"latitude just below -90":    {contents: "latitude = -90.0001\nlongitude = 0\n", key: "latitude"},
		"latitude far out":           {contents: "latitude = 1000\nlongitude = 0\n", key: "latitude"},
		"longitude just above 180":   {contents: "latitude = 0\nlongitude = 180.0001\n", key: "longitude"},
		"longitude just below -180":  {contents: "latitude = 0\nlongitude = -180.0001\n", key: "longitude"},
		"latitude is not a number":   {contents: "latitude = \"north\"\nlongitude = 0\n", key: "latitude"},
		"longitude is not a number":  {contents: "latitude = 0\nlongitude = true\n", key: "longitude"},
		"latitude is a nested table": {contents: "longitude = 0\n[latitude]\nvalue = 1\n", key: "latitude"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, test.contents)

			_, err := Load(path)
			if err == nil {
				t.Fatalf("Load accepted %s, want an error", name)
			}
			if !strings.Contains(err.Error(), test.key) {
				t.Errorf("error %q does not name the offending key %q", err, test.key)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error %q does not name the file %q", err, path)
			}
		})
	}
}

// The timezone is carried through untouched; validating it is later work.
func TestLoadCarriesTimezone(t *testing.T) {
	tests := map[string]struct {
		contents string
		timezone string
	}{
		"iana name": {
			contents: "latitude = 0\nlongitude = 0\ntimezone = \"Europe/London\"\n",
			timezone: "Europe/London",
		},
		"absent": {contents: "latitude = 0\nlongitude = 0\n"},
		"blank":  {contents: "latitude = 0\nlongitude = 0\ntimezone = \"\"\n"},
		"not validated here": {
			contents: "latitude = 0\nlongitude = 0\ntimezone = \"Mars/Olympus_Mons\"\n",
			timezone: "Mars/Olympus_Mons",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, test.contents)

			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load returned error %v, want nil", err)
			}
			if cfg.Timezone != test.timezone {
				t.Errorf("Timezone = %q, want %q", cfg.Timezone, test.timezone)
			}
		})
	}
}

// The file's extension does not decide the format; TOML is assumed (SUN-2).
func TestLoadReadsTOMLRegardlessOfExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "suntimes.conf")
	if err := os.WriteFile(path, []byte("latitude = 51.5\nlongitude = -0.12\n"), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error %v, want nil", err)
	}
	if cfg.Latitude != 51.5 || cfg.Longitude != -0.12 {
		t.Errorf("loaded %g,%g want 51.5,-0.12", cfg.Latitude, cfg.Longitude)
	}
}
