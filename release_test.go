package main

// Tests over what a release ships, where the failure mode is silent — nothing
// here breaks a build, so nothing here fails until someone downloads the
// artefact.
//
// Build settings live in two places: the Makefile, for local and
// cross-compiled builds, and .goreleaser.yaml, for the archives a release
// publishes. Most of these tests hold the two in step, because a release
// binary linked without the buildinfo ldflag still runs, it just reports
// "dev". The rest cover the files travelling alongside that binary (NFR-7).

import (
	"os"
	"os/exec"
	"path"
	"reflect"
	"regexp"
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

// goreleaserArchive is the slice of .goreleaser.yaml that decides which files
// travel alongside the binary in a release archive.
type goreleaserArchive struct {
	Files []string `yaml:"files"`
}

type goreleaserConfig struct {
	Builds   []goreleaserBuild   `yaml:"builds"`
	Archives []goreleaserArchive `yaml:"archives"`
}

func loadGoreleaserConfig(t *testing.T) goreleaserConfig {
	t.Helper()

	raw, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("reading .goreleaser.yaml: %v", err)
	}

	var cfg goreleaserConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parsing .goreleaser.yaml: %v", err)
	}
	return cfg
}

// onlyStanza returns the single stanza of a .goreleaser.yaml list. The tests
// below read index 0; a second stanza would leave them quietly covering half a
// release, so it fails loudly instead.
func onlyStanza[T any](t *testing.T, kind string, stanzas []T) T {
	t.Helper()

	if len(stanzas) != 1 {
		t.Fatalf("want exactly one %s stanza, got %d — the assertions here read "+
			"%ss[0] and would quietly stop covering the rest", kind, len(stanzas), kind)
	}
	return stanzas[0]
}

func loadGoreleaserBuild(t *testing.T) goreleaserBuild {
	t.Helper()

	return onlyStanza(t, "build", loadGoreleaserConfig(t).Builds)
}

func loadGoreleaserArchive(t *testing.T) goreleaserArchive {
	t.Helper()

	return onlyStanza(t, "archive", loadGoreleaserConfig(t).Archives)
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

// TestLicenseIsMIT checks the repo carries an MIT licence at the root, with a
// copyright line naming a holder and a year. Everything downstream reads it
// from there: GoReleaser packs it into the archives, and Homebrew, Scoop and
// the Linux package formats each want a licence to declare.
func TestLicenseIsMIT(t *testing.T) {
	raw, err := os.ReadFile("LICENSE")
	if err != nil {
		t.Fatalf("reading LICENSE: %v", err)
	}
	text := string(raw)

	if !strings.Contains(text, "MIT License") {
		t.Errorf("LICENSE heading = %q, want it to announce the MIT License",
			strings.SplitN(text, "\n", 2)[0])
	}

	// The permission grant, verbatim from the MIT text — a licence missing it
	// grants nothing, however MIT the heading claims to be.
	const grant = "Permission is hereby granted, free of charge, to any person obtaining a copy"
	if !strings.Contains(text, grant) {
		t.Errorf("LICENSE is missing the MIT permission grant %q", grant)
	}

	copyright := regexp.MustCompile(`Copyright \(c\) \d{4} \S`)
	if !copyright.MatchString(text) {
		t.Errorf("LICENSE has no line matching %v, want `Copyright (c) <year> <holder>`", copyright)
	}
}

// TestReleaseArchivesCarryLicense checks the licence actually ships (NFR-7).
// The archive stanza names its extra files by glob, so a LICENSE at the root
// travels only while some glob still matches it.
func TestReleaseArchivesCarryLicense(t *testing.T) {
	archive := loadGoreleaserArchive(t)

	for _, pattern := range archive.Files {
		ok, err := path.Match(pattern, "LICENSE")
		if err != nil {
			// Reported here rather than left to fall through as "no match",
			// which would blame the archive list for a broken pattern.
			t.Fatalf("archives[0].files pattern %q is malformed: %v", pattern, err)
		}
		if ok {
			return
		}
	}
	t.Errorf("archives[0].files = %v, want an entry matching LICENSE; "+
		"release archives would ship without it", archive.Files)
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
