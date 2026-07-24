package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/philipf/suntimes/internal/buildinfo"
)

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
