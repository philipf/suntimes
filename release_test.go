package main

// Build settings live in two places: the Makefile, for local and
// cross-compiled builds, and .goreleaser.yaml, for the archives a release
// publishes. These tests hold the two in step, because drift between them is
// silent — a release binary linked without the buildinfo ldflag still runs, it
// just reports "dev".

import (
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// versionTemplate is GoReleaser's placeholder for the version being built.
// The Makefile takes VERSION on the command line, so expanding its variables
// with this as VERSION makes the two files' version-bearing settings directly
// comparable.
const versionTemplate = "{{.Version}}"

// goreleaserBuild is the slice of .goreleaser.yaml that the Makefile also has
// an opinion about: how the binary is compiled, and for which platforms.
type goreleaserBuild struct {
	Env     []string            `yaml:"env"`
	Flags   []string            `yaml:"flags"`
	Ldflags []string            `yaml:"ldflags"`
	Goos    []string            `yaml:"goos"`
	Goarch  []string            `yaml:"goarch"`
	Ignore  []map[string]string `yaml:"ignore"`
}

type goreleaserConfig struct {
	Builds []goreleaserBuild `yaml:"builds"`
}

func loadGoreleaserBuild(t *testing.T) goreleaserBuild {
	t.Helper()

	raw, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("reading .goreleaser.yaml: %v", err)
	}

	var cfg goreleaserConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parsing .goreleaser.yaml: %v", err)
	}
	if len(cfg.Builds) != 1 {
		t.Fatalf("want exactly one build stanza, got %d — the assertions below "+
			"read builds[0] and would quietly stop covering the rest", len(cfg.Builds))
	}
	return cfg.Builds[0]
}

// makeVar expands a Makefile variable, with VERSION pinned to GoReleaser's
// template so version-bearing values compare equal. Where make is unavailable
// the check is skipped rather than failed: the Makefile is the reference, and
// a developer without make can still run the rest of the suite.
func makeVar(t *testing.T, name string) string {
	t.Helper()

	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not on PATH; skipping Makefile/GoReleaser drift check")
	}

	out, err := exec.Command("make", "--no-print-directory", "print-"+name,
		"VERSION="+versionTemplate).Output()
	if err != nil {
		t.Fatalf("make print-%s: %v", name, err)
	}
	return strings.TrimSpace(string(out))
}

// TestGoReleaserStampsBuildinfoVersion pins the one setting with a silent
// failure mode. GoReleaser's default ldflags stamp main.version, which this
// project does not read: the build succeeds, the archive looks right, and the
// binary inside reports "dev".
func TestGoReleaserStampsBuildinfoVersion(t *testing.T) {
	build := loadGoreleaserBuild(t)

	const want = "-s -w -X github.com/philipf/suntimes/internal/buildinfo.Version=" + versionTemplate
	if got := strings.Join(build.Ldflags, " "); got != want {
		t.Errorf("ldflags = %q, want %q", got, want)
	}
}

// TestGoReleaserBuildMirrorsMakefile checks the compiler settings agree. The
// Makefile is the reference: a release binary should be the same binary
// `make build` produces, only cross-compiled and archived.
func TestGoReleaserBuildMirrorsMakefile(t *testing.T) {
	build := loadGoreleaserBuild(t)

	tests := map[string]struct {
		makeName   string
		goreleaser []string
	}{
		"env":     {makeName: "GOENV", goreleaser: build.Env},
		"flags":   {makeName: "BUILDFLAGS", goreleaser: build.Flags},
		"ldflags": {makeName: "LDFLAGS", goreleaser: build.Ldflags},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Compared as whitespace-separated tokens, so a setting written as
			// one YAML entry and one written as several still compare equal.
			want := strings.Fields(makeVar(t, tc.makeName))
			got := strings.Fields(strings.Join(tc.goreleaser, " "))

			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s = %v, want %v (the Makefile's %s)", name, got, want, tc.makeName)
			}
		})
	}
}

// TestGoReleaserCoversMakefilePlatforms checks a release carries an archive for
// every platform the Makefile cross-compiles — the set NFR-5 names.
func TestGoReleaserCoversMakefilePlatforms(t *testing.T) {
	build := loadGoreleaserBuild(t)

	// An ignore list would subtract combinations from the cross product below,
	// and this test would go on passing while a platform quietly went missing.
	if len(build.Ignore) > 0 {
		t.Fatalf("build has an ignore list (%v); teach this test to subtract it", build.Ignore)
	}

	var got []string
	for _, goos := range build.Goos {
		for _, goarch := range build.Goarch {
			got = append(got, goos+"/"+goarch)
		}
	}
	want := strings.Fields(makeVar(t, "PLATFORMS"))

	slices.Sort(got)
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GoReleaser builds %v, the Makefile cross-compiles %v", got, want)
	}
}
