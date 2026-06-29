BINARY = tynet-cloud-init
# Convert hyphens to tildes so git-describe output (e.g. v0.1.0-3-gabc-dirty)
# is a valid Debian version. Fall back to 0.0.0~dev when no tag exists yet.
GIT_VERSION := $(shell git describe --tags --dirty 2>/dev/null | sed -e 's/^v//' -e 's/-/~/g')
VERSION ?= $(if $(GIT_VERSION),$(GIT_VERSION),0.0.0~dev)
IMAGE ?= ghcr.io/tya/tynet-cloud-init

.PHONY: help build build-linux deb image push-image test clean

.DEFAULT_GOAL := help

help:
	@echo "Targets:"
	@echo "  build         build for local machine"
	@echo "  build-linux   cross-compile for kickstart host (linux/arm64)"
	@echo "  deb           build linux/arm64 .deb package into dist/ (requires nfpm)"
	@echo "  image         build linux/arm64 container image as \$$(IMAGE):\$$(VERSION)"
	@echo "  push-image    build and push \$$(IMAGE):\$$(VERSION) and :latest (multi-arch via buildx)"
	@echo "  test          run unit tests against testdata/cloud-init/"
	@echo "  clean         remove built binary and dist/"

build:
	go build -o $(BINARY) .

build-linux:
	GOOS=linux GOARCH=arm64 go build -o $(BINARY) .

deb: build-linux
	@command -v nfpm >/dev/null || { echo "install nfpm: https://nfpm.goreleaser.com/install/"; exit 1; }
	mkdir -p dist
	# Generate a minimal Debian changelog at build time, gzipped to satisfy
	# lintian's `no-changelog` requirement for native packages. Keeps the
	# version pinned to whatever nfpm is about to package.
	{ \
	  echo "$(BINARY) ($(VERSION)) stable; urgency=low"; \
	  echo ""; \
	  echo "  * See https://github.com/tya/tynet-cloud-init/releases/tag/v$(VERSION) for details."; \
	  echo ""; \
	  echo " -- Ty Alexander <ty.alexander@gmail.com>  $$(LC_ALL=C date -u '+%a, %d %b %Y %H:%M:%S +0000')"; \
	} | gzip -9 -n > dist/changelog.gz
	VERSION=$(VERSION) nfpm package -f packaging/nfpm.yaml -p deb -t dist/

image:
	docker buildx build --platform linux/arm64 \
	  --load \
	  -t $(IMAGE):$(VERSION) \
	  .

push-image:
	docker buildx build --platform linux/arm64 \
	  --push \
	  -t $(IMAGE):$(VERSION) \
	  -t $(IMAGE):latest \
	  .

test:
	go test -v .

clean:
	rm -rf $(BINARY) dist
