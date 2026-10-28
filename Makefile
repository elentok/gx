.PHONY: build web-build web-dev web-dev-proxy web-test web-e2e web-types install test test-race test-short test-server e2e test-docker-ubuntu run demos demo-seed

GO_VERSION := 1.25.0

# A commit's detached auto maintenance can outlive a test and break its
# t.TempDir cleanup. Repos made by testutil carry the setting in their config;
# this covers packages that make repos with their own `git init`. Same as CI.
export GIT_CONFIG_COUNT := 1
export GIT_CONFIG_KEY_0 := maintenance.auto
export GIT_CONFIG_VALUE_0 := false

DEMO_DIR   := web/demo
DEMO_WORK  := $(DEMO_DIR)/.work
DEMO_TAPES := $(DEMO_DIR)/tapes

build: web-build
	go build -ldflags "-X github.com/elentok/gx/version.version=$(shell git describe --tags --always --dirty)" -o gx .

# webui/dist is not committed; the Go embed falls back to a placeholder without it.
web-build:
	cd webui && pnpm install --frozen-lockfile && pnpm build
	touch webui/dist/.gitkeep # vite empties dist; the Go embed needs this file to compile

# Serves the canned snapshot; GX_WEBUI_PROXY=1 proxies /v1 to 127.0.0.1:7421 instead.
web-dev:
	cd webui && pnpm install --frozen-lockfile && pnpm dev

# Serves the UI with hot reload and proxies /v1 to a running gx server on 127.0.0.1:7421.
web-dev-proxy:
	cd webui && pnpm install --frozen-lockfile && GX_WEBUI_PROXY=1 pnpm dev

web-test:
	cd webui && pnpm install --frozen-lockfile && pnpm exec tsc --noEmit && pnpm test

# Browser flows against fixture servers (webui/e2e/CATALOG.md); the gx binary embeds the built UI.
web-e2e: web-build
	cd webui && pnpm exec playwright install chromium && pnpm e2e

# Regenerates webui/src/generated/server.ts from the server's wire types.
web-types:
	go run github.com/gzuidhof/tygo@v0.2.21 generate

install: web-build
	go install -ldflags "-X github.com/elentok/gx/version.version=$(shell git describe --tags --always --dirty)" .

# -timeout=10m (per package; the server package alone sums to ~100s): legitimate tests (incl. real-git/production-style) finish in a few seconds each
# locally, but CI runners' lower core counts give concurrency-sensitive tests (e.g. pause/resume
# against a permit cap) more room to legitimately run long; 2m proved too tight there.
# The real-herdr e2e tests skip here even inside herdr: they need GX_E2E=1 (see e2e).
# The PTY-runner packages also run under -race: they share a vt emulator with
# a background goroutine, and data races there only surface under the detector.
# herdrrunner runs under it too, for its concurrent use of the herdr fake.
RACE_PKGS := ./agentrunner/screen ./agentrunner/claudeui ./nativerunner ./claudedoctor ./agentrunner/herdrrunner ./testutil/herdrfake

# The server package alone is most of the runtime and can't use t.Parallel (its
# harness mutates PATH/HOME), so it runs sharded across processes instead; see
# scripts/test-shards.sh. Everything else runs as a normal `go test`.
test: test-race
	go test $$(go list ./... | grep -Fvx github.com/elentok/gx/server) -timeout=10m
	scripts/test-shards.sh ./server

# test-server runs only the sharded server package (TEST_SHARDS=N, default 5).
test-server:
	scripts/test-shards.sh ./server

# test-short skips the slow server package for the inner loop.
test-short:
	go test -short $$(go list ./... | grep -Fvx github.com/elentok/gx/server) -timeout=10m

test-race:
	go test -race $(RACE_PKGS) -timeout=10m

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
