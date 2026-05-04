BINARY = serve-cloud-init
# Convert hyphens to tildes so git-describe output (e.g. v0.1.0-3-gabc-dirty)
# is a valid Debian version. Fall back to 0.0.0~dev when no tag exists yet.
GIT_VERSION := $(shell git describe --tags --dirty 2>/dev/null | sed -e 's/^v//' -e 's/-/~/g')
VERSION ?= $(if $(GIT_VERSION),$(GIT_VERSION),0.0.0~dev)

.PHONY: help build build-linux deb test clean

.DEFAULT_GOAL := help

help:
	@echo "Targets:"
	@echo "  build         build for local machine"
	@echo "  build-linux   cross-compile for kickstart host (linux/arm64)"
	@echo "  deb           build linux/arm64 .deb package into dist/ (requires nfpm)"
	@echo "  test          run unit tests against testdata/cloud-init/"
	@echo "  clean         remove built binary and dist/"

build:
	go build -o $(BINARY) .

build-linux:
	GOOS=linux GOARCH=arm64 go build -o $(BINARY) .

deb: build-linux
	@command -v nfpm >/dev/null || { echo "install nfpm: https://nfpm.goreleaser.com/install/"; exit 1; }
	mkdir -p dist
	VERSION=$(VERSION) nfpm package -f packaging/nfpm.yaml -p deb -t dist/

test:
	go test -v .

clean:
	rm -rf $(BINARY) dist
