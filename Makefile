SHELL := /bin/sh

GOCACHE_DIR ?= /tmp/mairu-go-build-cache
ENV_FILE ?= .env.local
WAILS_CGO_LDFLAGS ?= -framework UniformTypeIdentifiers
WAILS ?= $(shell if command -v wails >/dev/null 2>&1; then command -v wails; elif [ -x "$$HOME/go/bin/wails" ]; then printf "%s/go/bin/wails" "$$HOME"; fi)
WAILS_COMPILER ?= $(CURDIR)/scripts/go-wails
NPM ?= $(shell command -v npm)

.PHONY: check-wails frontend-install frontend-build frontend-test dev build test

check-wails:
	@if [ -z "$(WAILS)" ]; then \
		echo "wails CLI が見つかりません。'go install github.com/wailsapp/wails/v2/cmd/wails@latest' を実行してください。"; \
		exit 1; \
	fi

frontend-install:
	cd frontend && $(NPM) install

frontend-build:
	cd frontend && $(NPM) run build

frontend-test:
	cd frontend && $(NPM) run test

dev: check-wails
	@set -a; [ -f "$(ENV_FILE)" ] && . "$(ENV_FILE)"; set +a; \
		GOCACHE=$(GOCACHE_DIR) $(WAILS) dev -compiler $(WAILS_COMPILER)

build: frontend-build
	mkdir -p build/bin
	@set -a; [ -f "$(ENV_FILE)" ] && . "$(ENV_FILE)"; set +a; \
		CGO_LDFLAGS='$(WAILS_CGO_LDFLAGS)' GOCACHE=$(GOCACHE_DIR) go build -buildvcs=false -tags desktop,wv2runtime.download,production -ldflags "-w -s" -o build/bin/mairu

test:
	@set -a; [ -f "$(ENV_FILE)" ] && . "$(ENV_FILE)"; set +a; \
		GOCACHE=$(GOCACHE_DIR) go test ./...
	$(MAKE) frontend-test
