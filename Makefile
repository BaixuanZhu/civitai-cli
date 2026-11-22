# civitai-cli Makefile — M1 base targets + M4 release infrastructure.
# Usage: make <target>   (requires GNU Make; bundled with Git Bash / MinGW / WSL)

# ---- Variables ----
BINARY  := civ
# PRODUCT prefixes the release artifacts (zip / setup exe), distinct from
# BINARY (the exe inside them): civitai-cli-windows-<arch>.zip contains civ.exe.
PRODUCT := civitai-cli
# Version: prefers git describe (tag or commit); falls back to the default.
# Injected via ldflags into internal/app.Version at build time; no version
# literal is hardcoded in the source.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
ifeq ($(VERSION),)
VERSION := 0.1.0-dev
endif

DIST   := dist
GOOS   ?= windows
# Target arch: defaults to the host (go env GOARCH); cross-compile ARM64 with
# make build GOARCH=arm64. Artifacts are stored per-arch (dist/<arch>/civ.exe)
# so dual-arch builds don't clobber each other.
GOARCH ?= $(shell go env GOARCH)
TARGET := $(DIST)/$(GOARCH)/$(BINARY).exe

# Reproducible-build flags: no CGO, trimmed paths, no VCS stamping; the version
# is injected explicitly so builds carry the git-describe stamp. CGO_ENABLED=0
# and -buildvcs=false pin the remaining nondeterminism (cgo toolchain paths,
# .git state) so the same source + toolchain always builds byte-identical exes.
LDFLAGS := -s -w -X civitai-cli/internal/app.Version=$(VERSION)
# Dev builds additionally inject Bootstrap=off: dist/ dev binaries skip the
# silent bootstrap at startup (no user PATH writes), keeping the environment
# clean; release builds (build-dist) omit this flag so PATH self-bootstrap runs.
LDFLAGS_DEV := $(LDFLAGS) -X civitai-cli/internal/app.Bootstrap=off
BUILD_FLAGS := -trimpath -buildvcs=false

# SOURCE_DATE_EPOCH pins zip entry timestamps to the commit time (fixed fallback
# outside git), so the same commit always zips to the same bytes on any machine
# — the reproducibility contract. Export it to override (reproducible-builds.org
# convention).
SOURCE_DATE_EPOCH ?= $(shell git log -1 --pretty=%ct 2>/dev/null || echo 946684800)

.PHONY: help build build-dist test test-race vet fmt tidy run clean zip installer installer-only release dist-all integration

help:
	@echo "civitai-cli build targets:"
	@echo "  build          dev build (Bootstrap=off, no PATH writes) -> $(TARGET)"
	@echo "  build-dist     release-flavor build (Bootstrap on) -> $(TARGET)"
	@echo "  test           run unit tests (zero network: httptest + fixtures)"
	@echo "  test-race      run unit tests with the race detector"
	@echo "  vet            run go vet"
	@echo "  fmt            gofmt -w ."
	@echo "  tidy           go mod tidy"
	@echo "  run            build then run: make run ARGS='search anima'"
	@echo "  zip            deterministic zip of the already-built exe (no rebuild)"
	@echo "  installer      build NSIS setup exe -> $(DIST)/$(PRODUCT)-windows-<arch>-setup.exe"
	@echo "  installer-only package setup exe from the already-built exe (no rebuild)"
	@echo "  release        build portable zip + checksums.txt -> $(DIST)/"
	@echo "  dist-all       build both release assets (current GOARCH)"
	@echo "  integration    online smoke vs live Civitai API (needs scripts/integration-test.sh)"
	@echo "  clean          remove $(DIST)/"

# ---- Build (dev: bootstrap off, no user PATH pollution) ----
build:
	@mkdir -p $(DIST)/$(GOARCH)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(BUILD_FLAGS) -ldflags "$(LDFLAGS_DEV)" -o $(TARGET) .

# ---- Build (release flavor: bootstrap on, for release packaging and install.ps1) ----
build-dist:
	@mkdir -p $(DIST)/$(GOARCH)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(TARGET) .

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

run: build
	./$(TARGET) $(ARGS)

clean:
	rm -rf $(DIST)

# ---- Deterministic zip of the already-built exe (no rebuild) ----
# Packages the existing dist/<arch>/civ.exe via scripts/zipfile, which pins
# every entry header field instead of Compress-Archive's local-time timestamps.
# The packager itself must run on the HOST arch: command-line make variables
# (GOARCH=arm64) are exported to recipes, which would cross-compile `go run`
# too and break on the runner (env -u strips them so go falls back to host defaults).
zip:
	@env -u GOOS -u GOARCH go run ./scripts/zipfile -out $(DIST)/$(PRODUCT)-windows-$(GOARCH).zip -epoch $(SOURCE_DATE_EPOCH) $(TARGET)

# ---- For releases: portable zip + checksums.txt ----
# checksums.txt uses GNU coreutils sha256sum format; file names carry no path
# prefix (matching upgrade's parseChecksum bare-filename convention). All zips
# under dist/ are listed, so building both arches and re-running this target
# merges them into one checksum manifest.
release: build-dist zip
	@cd $(DIST) && sha256sum $$(ls *.zip) > checksums.txt

# ---- NSIS installer (requires makensis on PATH: scoop install nsis / choco install nsis) ----
# Output: dist/civitai-cli-windows-$(GOARCH)-setup.exe (double-click setup
# wizard; upload alongside the portable zip on the GitHub Release). The
# installer stub is x86 and runs under emulation on ARM64, while dropping the
# native $(GOARCH) exe.
installer: build-dist installer-only

# Package the setup exe from the already-built exe (no rebuild), so CI can
# build once, then package both artifacts from the same build. The /D flags
# are not POSIX paths, so MSYS_NO_PATHCONV=1 stops Git Bash / MSYS from mangling them.
installer-only:
	@MSYS_NO_PATHCONV=1 makensis /DAPP_VERSION=$(VERSION) /DAPP_ARCH=$(GOARCH) installer/$(PRODUCT).nsi

# ---- One-shot: build all release assets for the current GOARCH ----
dist-all: installer release

# ---- Online integration smoke (live Civitai API, anonymous) ----
# The script exports CIV_NO_BOOTSTRAP=1 / CIV_NO_CACHE=1 itself, so the test
# binary never touches the user PATH/registry or replays cached responses
# (cache replay would mask upstream API drift — this is the drift canary).
# NOTE: scripts/integration-test.sh lands after M3 (sync command); until then
# this target fails at the bash step.
integration: build
	@bash scripts/integration-test.sh $(TARGET)