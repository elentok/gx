.PHONY: build web-build web-dev web-test web-lint web-e2e web-types install test test-docker-ubuntu run

GO_VERSION := 1.25.0

# Runs `gx brief`. CI has no installed gx, and an installed one can be older than the checkout.
GX ?= go run .

# The second -X stamps this checkout's directory so gx can find its Dev checkout.
MAKEFILE_DIR := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
LDFLAGS = -X github.com/elentok/gx/version.version=$(shell git describe --tags --always --dirty) -X github.com/elentok/gx/version.DevDir=$(MAKEFILE_DIR)

build: web-build
	go build -ldflags "$(LDFLAGS)" -o gx .

# webui/dist is not committed; the Go embed falls back to a placeholder without it.
web-build:
	cd webui && pnpm install --frozen-lockfile && GX_WEB_VERSION=$(shell git describe --tags --always --dirty) pnpm build
	touch webui/dist/.gitkeep # vite empties dist; the Go embed needs this file to compile

# Serves the canned snapshot. Dev mode in gx runs its own Vite; this target is only the canned snapshot.
web-dev:
	cd webui && pnpm install --frozen-lockfile && pnpm dev

# WEB_TEST_WORKERS caps vitest's workers; scripts/test.sh sets it to share the CPU budget.
WEB_TEST_WORKERS ?=

web-lint:
	cd webui && pnpm install --frozen-lockfile
	$(GX) brief -C webui biome

web-test:
	cd webui && pnpm install --frozen-lockfile
	$(GX) brief -C webui tsc
	$(GX) brief -C webui vitest $(if $(WEB_TEST_WORKERS),--maxWorkers=$(WEB_TEST_WORKERS))

# Browser flows against fixture servers (webui/e2e/CATALOG.md); the gx binary embeds the built UI.
web-e2e: web-build
	cd webui && pnpm exec playwright install chromium
	$(GX) brief -C webui playwright $(if $(ARGS),$(ARGS),--all)

# Regenerates webui/src/generated/server.ts and ops.ts from the server's wire types.
web-types:
	go run github.com/gzuidhof/tygo@v0.2.21 generate

install: web-build
	go install -ldflags "$(LDFLAGS)" .

# Every non-e2e test step side by side; see scripts/test.sh.
test:
	scripts/test.sh

coverage:
	go test -exec "$(CURDIR)/scripts/proc-cap" ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

test-docker-ubuntu:
	docker run --rm \
		-v $(CURDIR):/work \
		-w /work \
		-e GO_VERSION=$(GO_VERSION) \
		ubuntu:24.04 \
		bash ./scripts/run-tests-inside-docker-ubuntu.sh

run:
	go run .
