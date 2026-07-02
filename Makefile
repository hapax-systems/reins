# Reins — engine Makefile. Instance config: ~/.config/reins/config.toml (or $REINS_CONFIG).
# Per-machine overrides (e.g. PY = a venv with fastapi+uvicorn+the substrate) go in reins.local.mk.
-include reins.local.mk

PY ?= python3   # python for the READ API; needs fastapi+uvicorn + the substrate importable

# Port + paths come from the instance config (config.toml api_url) — ONE source of truth. Both halves
# self-resolve it; the Makefile does not inject a port (which would fight the config).

.PHONY: up run api build install test smoke drive avsdlc fmt tidy help

PREFIX ?= $(HOME)/.local
# the ONE semver source (repo VERSION file), stamped into the Go binary (-X main.version) and read by the
# Python /read/meta (serving_version). $(strip) drops the trailing newline so the ldflag is clean.
VERSION := $(strip $(shell cat $(dir $(lastword $(MAKEFILE_LIST)))VERSION 2>/dev/null || echo dev))

help: ## list targets
	@grep -E '^[a-z]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*## /\t/' | sort

up: ## bring up BOTH halves — READ API in the background, then the cockpit; API torn down on exit
	@$(PY) api/reins_read.py >/tmp/reins-api.log 2>&1 & \
	  api_pid=$$!; \
	  trap 'kill $$api_pid 2>/dev/null' EXIT INT TERM; \
	  printf 'reins: READ API pid %s (port from config.toml; log /tmp/reins-api.log)\n' "$$api_pid"; \
	  sleep 1; \
	  go run ./cmd/reins

run: ## the cockpit only (assumes the READ API is already up)
	go run ./cmd/reins

api: ## the READ API only (foreground; port from config.toml)
	$(PY) api/reins_read.py

build: ## build the cockpit binary -> bin/reins (VERSION-stamped)
	go build -ldflags "-X main.version=$(VERSION)" -o bin/reins ./cmd/reins

install: build ## install the cockpit -> $(PREFIX)/bin/reins (on PATH)
	@mkdir -p $(PREFIX)/bin
	@# rename-over, never cp-over: a RUNNING cockpit holds the old inode (cp fails ETXTBSY mid-session);
	@# rename swaps the path atomically while live sessions keep their inode until exit
	@cp bin/reins $(PREFIX)/bin/.reins.staged && mv -f $(PREFIX)/bin/.reins.staged $(PREFIX)/bin/reins
	@printf 'reins: installed -> %s/bin/reins\n' "$(PREFIX)"

test: ## go + python test suites
	go test ./...
	cd api && $(PY) -m pytest -q

smoke: ## headless NAV smoke — visits every page, no panic, on-air redaction
	go test ./internal/smoke/ -v

drive: ## drive a nav sequence headless + print the frame (SPEC=":coordinator; j; v"  [SIZE=170x46] [FLAGS=--air])
	go run ./cmd/reins --drive "$(SPEC)" size:$(or $(SIZE),170x46) $(FLAGS)

demo: ## run the seed-backed FIXTURE cockpit (no estate, no API, no spine — a stranger's first look)
	go run -ldflags "-X main.version=$(VERSION)" ./cmd/reins --demo

kit: ## install reins from source (cockpit + config) — the one-command install path (PREFIX=~/.local)
	sh scripts/install.sh

avsdlc: ## render + AVSDLC-confirm every pane with an intent (visual regression; --live optional via FLAGS=--live)
	bash scripts/reins-avsdlc-suite.sh $(FLAGS)

fmt: ## gofmt
	go fmt ./...

tidy: ## go mod tidy
	go mod tidy
