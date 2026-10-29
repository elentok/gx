.PHONY: build web-build web-dev web-test web-e2e web-types install test e2e test-docker-ubuntu run demos demo-seed

GO_VERSION := 1.25.0

# Runs `gx brief`. CI has no installed gx, and an installed one can be older than the checkout.
GX ?= go run .

DEMO_DIR   := web/demo
DEMO_WORK  := $(DEMO_DIR)/.work
DEMO_TAPES := $(DEMO_DIR)/tapes

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

# demo-seed rebuilds the deterministic throwaway repo the demos render against.
demo-seed:
	bash $(DEMO_DIR)/seed.sh $(DEMO_WORK)

# demos regenerates all demo GIFs into docs/ (single source for README + web/).
# Builds gx, then renders every VHS tape — re-seeding the fixture before each one
# so the tapes (which stage hunks, reword, create worktrees, …) always start from
# a clean, deterministic state regardless of order. Requires vhs + ttyd and the
# "Agave Nerd Font" installed. The kitty image-diff demo is captured separately
# via web/demo/image-diff.sh (VHS can't render the kitty graphics protocol).
demos: build
	@if ls $(DEMO_TAPES)/*.tape >/dev/null 2>&1; then \
		for tape in $(DEMO_TAPES)/*.tape; do \
			echo "==> seeding fixture"; \
			bash $(DEMO_DIR)/seed.sh $(DEMO_WORK) >/dev/null; \
			echo "==> vhs $$tape"; \
			PATH="$(CURDIR):$$PATH" vhs "$$tape"; \
		done; \
	else \
		echo "no tapes in $(DEMO_TAPES) yet — see beads main-3vg.3"; \
	fi
