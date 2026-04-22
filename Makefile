GO ?= go
GOCACHE ?= $(CURDIR)/.cache/gocache
BINARY ?= gm
MAIN_PKG ?= ./cmd/gm

.PHONY: help build test fmt clean install

help:
	@printf '%s\n' \
		'Available targets:' \
		'  make build    Build the gm binary' \
		'  make test     Run the test suite' \
		'  make fmt      Format Go source files' \
		'  make install  Install the binary with go install' \
		'  make clean    Remove build artifacts and local caches'

build:
	@mkdir -p "$(dir $(GOCACHE))"
	GOCACHE="$(GOCACHE)" $(GO) build -trimpath -o "$(BINARY)" $(MAIN_PKG)

test:
	@mkdir -p "$(dir $(GOCACHE))"
	GOCACHE="$(GOCACHE)" $(GO) test ./...

fmt:
	GOCACHE="$(GOCACHE)" $(GO) fmt ./...

install:
	GOCACHE="$(GOCACHE)" $(GO) install $(MAIN_PKG)

clean:
	rm -rf "$(BINARY)" .cache
