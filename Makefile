BINARY = serve-cloud-init

.PHONY: help build build-linux test clean

.DEFAULT_GOAL := help

help:
	@echo "Targets:"
	@echo "  build         build for local machine"
	@echo "  build-linux   cross-compile for kickstart host (linux/arm64)"
	@echo "  test          run unit tests against testdata/cloud-init/"
	@echo "  clean         remove built binary"

build:
	go build -o $(BINARY) .

build-linux:
	GOOS=linux GOARCH=arm64 go build -o $(BINARY) .

test:
	go test -v .

clean:
	rm -f $(BINARY)
