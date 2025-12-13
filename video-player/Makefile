SHELL := /bin/bash

APP := pp
PKG := ./cmd/pp
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP)

# Install location (default: user-local, no sudo needed)
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

# Workaround for environments where the default Go cache is not writable.
# Override to empty to use system defaults:
#   make build GOCACHE= GOPATH=
GOCACHE ?= $(CURDIR)/.gocache
GOPATH ?= $(CURDIR)/.gopath

.PHONY: help build install uninstall clean test

help:
	@echo "Targets:"
	@echo "  make build                 Build ./$(BIN)"
	@echo "  make install               Install to $(BINDIR)/$(APP)"
	@echo "  make uninstall             Remove $(BINDIR)/$(APP)"
	@echo "  make test                  Run go test"
	@echo "  make clean                 Remove build artifacts"
	@echo ""
	@echo "Vars:"
	@echo "  PREFIX=/usr/local          Install prefix"
	@echo "  BINDIR=/custom/bin         Install bin dir"
	@echo "  GOCACHE= GOPATH=           Use system Go cache"

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build: $(BIN_DIR)
	env GOCACHE="$(GOCACHE)" GOPATH="$(GOPATH)" go build -o "$(BIN)" "$(PKG)"

install: build
	mkdir -p "$(BINDIR)"
	install -m 0755 "$(BIN)" "$(BINDIR)/$(APP)"
	@echo "Installed: $(BINDIR)/$(APP)"

uninstall:
	rm -f "$(BINDIR)/$(APP)"
	@echo "Removed: $(BINDIR)/$(APP)"

test:
	env GOCACHE="$(GOCACHE)" GOPATH="$(GOPATH)" go test ./...

clean:
	rm -rf "$(BIN_DIR)" .gocache .gopath

