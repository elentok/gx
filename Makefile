.PHONY: build web-build web-dev web-test web-e2e web-types install test e2e test-docker-ubuntu run

GO_VERSION := 1.25.0

# Runs `gx brief`. CI has no installed gx, and an installed one can be older than the checkout.
GX ?= go run .

build: web-build
	go build -ldflags "-X github.com/elentok/gx/version.version=$(shell git describe --tags --always --dirty)" -o gx .

# webui/dist is not committed; the Go embed falls back to a placeholder without it.
web-build:
	cd webui && pnpm install --frozen-lockfile && pnpm build
	touch webui/dist/.gitkeep # vite empties dist; the Go embed needs this file to compile

# Serves the canned snapshot. Against a real server, set server.web.dev-url instead.
web-dev:
	cd webui && pnpm install --frozen-lockfile && pnpm dev

# WEB_TEST_WORKERS caps vitest's workers; scripts/test.sh sets it to share the CPU budget.
WEB_TEST_WORKERS ?=

web-test:
	cd webui && pnpm install --frozen-lockfile
	$(GX) brief -C webui tsc
	$(GX) brief -C webui vitest $(if $(WEB_TEST_WORKERS),--maxWorkers=$(WEB_TEST_WORKERS))

# Browser flows against fixture servers (webui/e2e/CATALOG.md); the gx binary embeds the built UI.
web-e2e: web-build
	cd webui && pnpm exec playwright install chromium
	$(GX) brief -C webui playwright

# Regenerates webui/src/generated/server.ts from the server's wire types.
web-types:
	go run github.com/gzuidhof/tygo@v0.2.21 generate

install: web-build
	go install -ldflags "-X github.com/elentok/gx/version.version=$(shell git describe --tags --always --dirty)" .

# Every non-e2e test step side by side; see scripts/test.sh. The real-herdr e2e
# tests skip here even inside herdr: they need GX_E2E=1 (see e2e).
test:
	scripts/test.sh

# e2e runs the herdr e2e suite against a real, already-running herdr daemon
# (herdr must be on PATH). Tests skip themselves if either isn't available.
e2e:
	GX_E2E=1 HERDR_ENV=1 go test ./e2e/... -timeout=2m

coverage:
	go test ./... -coverprofile=coverage.out
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
