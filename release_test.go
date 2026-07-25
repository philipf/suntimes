package main

// Tests over what a release ships, where the failure mode is silent — nothing
// here breaks a build, so nothing here fails until someone downloads the
// artefact.
//
// Build settings live in two places: the Makefile, for local and
// cross-compiled builds, and .goreleaser.yaml, for the archives a release
// publishes. Most of these tests hold the two in step, because a release
// binary linked without the buildinfo ldflag still runs, it just reports
// "dev". The rest cover the files travelling alongside that binary (NFR-7)
// and the workflow that publishes them, where a mistake surfaces only on the
// one push a year that cuts a release.

import (
	"fmt"
	"maps"
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

// goreleaserChangelog is the stanza deciding what a published release says
// happened since the last one. `disable` takes a bool or a template string, so
// it is read as neither.
type goreleaserChangelog struct {
	Disable any    `yaml:"disable"`
	Use     string `yaml:"use"`
}

type goreleaserConfig struct {
	Builds    []goreleaserBuild   `yaml:"builds"`
	Archives  []goreleaserArchive `yaml:"archives"`
	Changelog goreleaserChangelog `yaml:"changelog"`
}

// loadYAML decodes one of the repo's configuration files into T, which declares
// only the fields the tests have an opinion about — everything else in the file
// is ignored rather than having to be modelled.
func loadYAML[T any](t *testing.T, path string) T {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var decoded T
	if err := yaml.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return decoded
}

func loadGoreleaserConfig(t *testing.T) goreleaserConfig {
	t.Helper()

	return loadYAML[goreleaserConfig](t, ".goreleaser.yaml")
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

// matchesAny reports whether name matches any of a configured list of
// shell-style patterns, where what names the list for the failure message. A
// malformed pattern fails the test where it is written, rather than falling
// through as "no match" and blaming the list for not covering name.
func matchesAny(t *testing.T, what string, patterns []string, name string) bool {
	t.Helper()

	return slices.ContainsFunc(patterns, func(pattern string) bool {
		matched, err := path.Match(pattern, name)
		if err != nil {
			t.Fatalf("%s pattern %q is malformed: %v", what, pattern, err)
		}
		return matched
	})
}

func loadGoreleaserBuild(t *testing.T) goreleaserBuild {
	t.Helper()

	return onlyStanza(t, "build", loadGoreleaserConfig(t).Builds)
}

func loadGoreleaserArchive(t *testing.T) goreleaserArchive {
	t.Helper()

	return onlyStanza(t, "archive", loadGoreleaserConfig(t).Archives)
}

// workflowStep is one step of a workflow job: which action it runs, and the
// inputs and environment it runs with. Anything stricter than this — the
// runner, the Go version, step order — is CI's own business and changes
// without a release breaking.
type workflowStep struct {
	Uses string            `yaml:"uses"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

// workflowJob is one job of a workflow, with the permissions it grants its
// token.
type workflowJob struct {
	Permissions map[string]string `yaml:"permissions"`
	Steps       []workflowStep    `yaml:"steps"`
}

// releaseWorkflow is the release workflow, as much of it as the tests below
// assert on. It decodes `permissions` as a mapping, the form both GitHub and
// this repo's workflows use. Written as a bare scalar (`permissions: write-all`)
// it fails to parse, and loadReleaseWorkflow reports that as the parse error it
// is rather than as a missing permission.
type releaseWorkflow struct {
	On struct {
		Push struct {
			Branches []string `yaml:"branches"`
			Tags     []string `yaml:"tags"`
		} `yaml:"push"`
	} `yaml:"on"`
	Permissions map[string]string      `yaml:"permissions"`
	Jobs        map[string]workflowJob `yaml:"jobs"`
}

const releaseWorkflowPath = ".github/workflows/release.yml"

func loadReleaseWorkflow(t *testing.T) releaseWorkflow {
	t.Helper()

	return loadYAML[releaseWorkflow](t, releaseWorkflowPath)
}

// onlyJob returns the workflow's single job. As with onlyStanza — which does
// the counting — a second job would leave the assertions below covering only
// whichever one came back.
func onlyJob(t *testing.T, wf releaseWorkflow) workflowJob {
	t.Helper()

	return onlyStanza(t, "job", slices.Collect(maps.Values(wf.Jobs)))
}

// stepUsing returns the first step using the named action, matched on the part
// before the `@version` so a version bump doesn't look like a deleted step.
func stepUsing(t *testing.T, job workflowJob, action string) workflowStep {
	t.Helper()

	for _, step := range job.Steps {
		if strings.SplitN(step.Uses, "@", 2)[0] == action {
			return step
		}
	}
	t.Fatalf("no step in %s uses %s", releaseWorkflowPath, action)
	return workflowStep{}
}

// TestReleaseWorkflowRunsOnVersionTagsOnly checks the trigger. Both failure
// modes here are quiet: a pattern that misses `v1.2.3` means a tag push
// publishes nothing at all, and a branch trigger means every merge to main
// tries to publish a release from an untagged commit.
func TestReleaseWorkflowRunsOnVersionTagsOnly(t *testing.T) {
	wf := loadReleaseWorkflow(t)
	push := wf.On.Push

	if len(push.Branches) > 0 {
		t.Errorf("on.push.branches = %v, want none; releases are cut from tags, not branches",
			push.Branches)
	}

	const tag = "v1.2.3"
	if !matchesAny(t, "on.push.tags", push.Tags, tag) {
		t.Errorf("on.push.tags = %v, want a pattern matching %q", push.Tags, tag)
	}
}

// TestReleaseWorkflowCheckoutIsNotShallow pins fetch-depth. Actions checks out
// a single commit by default, and GoReleaser derives the version and the
// changelog from the tags and commits that clone does not have.
func TestReleaseWorkflowCheckoutIsNotShallow(t *testing.T) {
	checkout := stepUsing(t, onlyJob(t, loadReleaseWorkflow(t)), "actions/checkout")

	// Compared as text: YAML reads `0` as an int and `"0"` as a string, and
	// either is a full clone as far as the action is concerned.
	if got := fmt.Sprint(checkout.With["fetch-depth"]); got != "0" {
		t.Errorf("checkout fetch-depth = %v, want 0 (full history and tags)", got)
	}
}

// TestReleaseWorkflowMayPublishARelease checks the token can write. The default
// GITHUB_TOKEN is read-only for contents, which lets the whole build succeed
// and fails only at the upload.
func TestReleaseWorkflowMayPublishARelease(t *testing.T) {
	wf := loadReleaseWorkflow(t)

	// Job-level permissions replace the workflow-level block outright rather
	// than merging into it, so the job's own is what the token gets.
	permissions := onlyJob(t, wf).Permissions
	if permissions == nil {
		permissions = wf.Permissions
	}

	if got := permissions["contents"]; got != "write" {
		t.Errorf("contents permission = %q, want %q; publishing a release writes tags and assets",
			got, "write")
	}
}

// TestReleaseWorkflowPublishes checks the workflow actually releases. A
// `--snapshot` run here would go green having published nothing, which is
// exactly what a release that failed to run looks like from the outside.
//
// The token is the other half: the permission above grants the write, and this
// step is where GoReleaser is handed the credential to use it. Dropping the env
// block leaves the whole build green and fails at the upload.
func TestReleaseWorkflowPublishes(t *testing.T) {
	step := stepUsing(t, onlyJob(t, loadReleaseWorkflow(t)), "goreleaser/goreleaser-action")

	args := strings.Fields(fmt.Sprint(step.With["args"]))
	if len(args) == 0 || args[0] != "release" {
		t.Errorf("goreleaser args = %v, want them to start with `release`", args)
	}
	for _, arg := range args {
		if arg == "--snapshot" || arg == "--skip=publish" || arg == "--skip-publish" {
			t.Errorf("goreleaser args = %v, contains %q; the run would publish nothing", args, arg)
		}
	}

	if _, ok := step.Env["GITHUB_TOKEN"]; !ok {
		t.Errorf("goreleaser step env = %v, want a GITHUB_TOKEN; "+
			"without one GoReleaser has nothing to authenticate the upload with", step.Env)
	}
}

// TestGoReleaserWritesReleaseNotesFromGitLog checks release notes get written
// at all, and from the local git log — the reason the checkout above is a full
// clone. A disabled changelog publishes a release with an empty body.
func TestGoReleaserWritesReleaseNotesFromGitLog(t *testing.T) {
	changelog := loadGoreleaserConfig(t).Changelog

	if disabled, ok := changelog.Disable.(bool); ok && disabled {
		t.Error("changelog.disable is true, want release notes generated from the commits")
	} else if !ok && changelog.Disable != nil {
		t.Errorf("changelog.disable = %v, want it unset", changelog.Disable)
	}

	if changelog.Use != "git" {
		t.Errorf("changelog.use = %q, want %q (the commits since the previous tag)",
			changelog.Use, "git")
	}
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

	if !matchesAny(t, "archives[0].files", archive.Files, "LICENSE") {
		t.Errorf("archives[0].files = %v, want an entry matching LICENSE; "+
			"release archives would ship without it", archive.Files)
	}
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
