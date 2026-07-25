# Release automation and packaging are in scope, driven by GoReleaser

`docs/PRD.md` §3 originally listed "release automation / packaging beyond `go build` and a
`Makefile`" as a v1 non-goal, which left `go install` or clone-and-`make build` as the only
ways in — both requiring a Go toolchain the target user may not have. That non-goal is
withdrawn: shipping pre-built binaries is now part of v1 (PRD NFR-7 and NFR-8), and
GoReleaser is the mechanism — a tagged push builds all six targets, archives each with the
README and LICENSE, writes a checksums file, and publishes the release. Hand-rolled
`Makefile` release targets plus `gh release create` were rejected because every channel
below is a first-class GoReleaser output, and reimplementing each by hand is the bulk of
the work.

**Distribution channels being adopted**, to be fed from that same GoReleaser run: GitHub
Releases as the source of truth for artefacts (#11), a Homebrew tap `philipf/tap` (#12), a
`curl | sh` install script (#13), Scoop (#14) and winget (#16) on Windows, deb, rpm and apk
packages (#15), and an AUR `suntimes-bin` package (#17). This ADR records the decision to
adopt them; none are wired up yet, and each lands with its own issue.

## Consequences

- The repository needs an explicit licence, which packaging metadata declares — hence the
  MIT `LICENSE` at the root. Homebrew, Scoop, winget and the Linux package formats all
  carry a licence field.
- Releases become tag-driven. A tag is a published artefact set, and unpublishing one is
  awkward.
- `.goreleaser.yaml` becomes the single description of what a release contains and where it
  goes. New channels are added there, not as separate CI steps. The Makefile keeps its own
  cross-compilation path regardless (NFR-5), and `release_test.go` holds the two files'
  build settings in step.
