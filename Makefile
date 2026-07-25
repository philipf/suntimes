# suntimes — build tooling (NFR-1, NFR-5)
#
# All binaries are built with CGO_ENABLED=0 so they are statically linked and
# have no runtime dependencies.

BINARY  := suntimes
PKG     := ./...
MODULE  := github.com/philipf/suntimes
DIST    := dist

# Version stamped into the binary. Derived from git when available, otherwise
# "dev". Override on the command line: make build VERSION=v1.0.0
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(MODULE)/internal/buildinfo.Version=$(VERSION)

GO      := go
GOENV   := CGO_ENABLED=0
# -buildvcs=false: the version is stamped explicitly via -ldflags above, so the
# build does not need Go's VCS metadata. Disabling it also keeps builds working
# in git worktree checkouts, where Go can misidentify the repository root.
BUILDFLAGS := -trimpath -buildvcs=false
GOBUILD := $(GOENV) $(GO) build $(BUILDFLAGS) -ldflags "$(LDFLAGS)"

# GoReleaser (see .goreleaser.yaml) produces the release archives, and repeats
# GOENV, BUILDFLAGS and LDFLAGS so a released binary is the binary `make build`
# produces. Change them here and there together: release_test.go fails if the
# two drift apart.
GORELEASER ?= goreleaser

# host os/arch × the platforms we cross-compile for
PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

.DEFAULT_GOAL := build

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_%/-]+:.*?## ' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the binary for the host platform
	$(GOBUILD) -o $(BINARY) .

.PHONY: install
install: ## Install the binary into GOBIN
	$(GOENV) $(GO) install $(BUILDFLAGS) -ldflags "$(LDFLAGS)" .

.PHONY: test
test: ## Run the tests
	$(GO) test $(PKG)

.PHONY: vet
vet: ## Run go vet
	$(GO) vet $(PKG)

.PHONY: fmt
fmt: ## Format all Go source
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if any Go source is unformatted
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed for:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: check
# CI runs these prerequisites as separate steps, so a failure names which check
# broke. Add a target here and add it to .github/workflows/ci.yml too.
check: fmt-check vet test ## Run formatting, vet and tests

.PHONY: tidy
tidy: ## Tidy go.mod/go.sum
	$(GO) mod tidy

# Cross-compilation. `make dist/suntimes_linux_arm64` builds a single target;
# `make cross` builds them all into $(DIST)/.
.PHONY: cross
cross: $(addprefix $(DIST)/$(BINARY)_,$(subst /,_,$(PLATFORMS))) ## Cross-compile every supported platform

# Windows binaries need the .exe suffix.
$(DIST)/$(BINARY)_windows_%:
	@mkdir -p $(DIST)
	GOOS=windows GOARCH=$* $(GOBUILD) -o $@.exe .

$(DIST)/$(BINARY)_%:
	@mkdir -p $(DIST)
	GOOS=$(word 1,$(subst _, ,$*)) GOARCH=$(word 2,$(subst _, ,$*)) $(GOBUILD) -o $@ .

# Convenience aliases, one per platform.
.PHONY: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64 build-windows-arm64
build-linux-amd64:   $(DIST)/$(BINARY)_linux_amd64   ## Cross-compile for linux/amd64
build-linux-arm64:   $(DIST)/$(BINARY)_linux_arm64   ## Cross-compile for linux/arm64
build-darwin-amd64:  $(DIST)/$(BINARY)_darwin_amd64  ## Cross-compile for darwin/amd64
build-darwin-arm64:  $(DIST)/$(BINARY)_darwin_arm64  ## Cross-compile for darwin/arm64
build-windows-amd64: $(DIST)/$(BINARY)_windows_amd64 ## Cross-compile for windows/amd64
build-windows-arm64: $(DIST)/$(BINARY)_windows_arm64 ## Cross-compile for windows/arm64

# Release artefacts. `make cross` above compiles bare binaries for a quick
# local check and needs nothing but Go; these targets need GoReleaser v2
# (go install github.com/goreleaser/goreleaser/v2@latest) and produce what a
# release actually publishes — archives and a checksums file. Both write to
# $(DIST) — .goreleaser.yaml sets `dist` to match — and --clean empties it
# first, so a snapshot discards any binaries `make cross` left behind.
.PHONY: snapshot
snapshot: ## Build release archives and checksums locally, without tagging or publishing
	$(GORELEASER) release --snapshot --clean

.PHONY: release-check
release-check: ## Validate .goreleaser.yaml
	$(GORELEASER) check

# Expand a single variable, e.g. `make print-LDFLAGS`. Used by release_test.go
# to compare the Makefile's build settings against .goreleaser.yaml. Not .PHONY:
# make matches .PHONY prerequisites literally, so a pattern there would declare
# a target named "print-%" and do nothing for print-LDFLAGS.
print-%:
	@echo '$($*)'

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf $(DIST) $(BINARY)
