SHELL := /bin/sh

GOCACHE_DIR ?= /tmp/mairu-go-build-cache
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
  WAILS_CGO_LDFLAGS ?= -framework UniformTypeIdentifiers
else
  WAILS_CGO_LDFLAGS ?=
endif
WAILS ?= $(shell if command -v wails >/dev/null 2>&1; then command -v wails; elif [ -x "$$HOME/go/bin/wails" ]; then printf "%s/go/bin/wails" "$$HOME"; fi)
WAILS_COMPILER ?= $(CURDIR)/scripts/go-wails
NPM ?= $(shell command -v npm)

.PHONY: all check-wails check-npm frontend-install frontend-build frontend-test dev build test clean

all: build

check-npm:
	@if [ -z "$(NPM)" ]; then \
		echo "npm が見つかりません。Node.js をインストールしてください。"; \
		exit 1; \
	fi

check-wails:
	@if [ -z "$(WAILS)" ]; then \
		echo "wails CLI が見つかりません。'go install github.com/wailsapp/wails/v2/cmd/wails@latest' を実行してください。"; \
		exit 1; \
	fi

frontend-install: check-npm
	cd frontend && $(NPM) install

frontend-build: frontend-install
	cd frontend && $(NPM) run build

frontend-test: check-npm
	cd frontend && $(NPM) run test

dev: check-wails
	GOCACHE=$(GOCACHE_DIR) $(WAILS) dev -compiler $(WAILS_COMPILER)

build: frontend-build
	mkdir -p build/bin
	CGO_LDFLAGS='$(WAILS_CGO_LDFLAGS)' GOCACHE=$(GOCACHE_DIR) go build -buildvcs=false -tags desktop,wv2runtime.download,production -ldflags "-w -s" -o build/bin/mairu

test:
	GOCACHE=$(GOCACHE_DIR) go test ./...
	$(MAKE) frontend-test

clean:
	rm -rf build/bin
	cd frontend && rm -rf node_modules dist
