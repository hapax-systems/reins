# Reins — engine Makefile. Instance config: ~/.config/reins/config.toml (or $REINS_CONFIG).
# Per-machine overrides (e.g. PY = a venv with fastapi+uvicorn+the substrate) go in reins.local.mk.
-include reins.local.mk

PY ?= python3   # python for the READ API; needs fastapi+uvicorn + the substrate importable

# Port + paths come from the instance config (config.toml api_url) — ONE source of truth. Both halves
# self-resolve it; the Makefile does not inject a port (which would fight the config).

.PHONY: up run api build test fmt tidy help

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

build: ## build the cockpit binary -> bin/reins
	go build -o bin/reins ./cmd/reins

test: ## go + python test suites
	go test ./...
	cd api && $(PY) -m pytest -q

fmt: ## gofmt
	go fmt ./...

tidy: ## go mod tidy
	go mod tidy
