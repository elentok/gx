.PHONY: build install test e2e test-docker-ubuntu run demos demo-seed

GO_VERSION := 1.25.0

DEMO_DIR   := web/demo
DEMO_WORK  := $(DEMO_DIR)/.work
DEMO_TAPES := $(DEMO_DIR)/tapes

build:
	go build -ldflags "-X github.com/elentok/gx/cmd.version=$(shell git describe --tags --always --dirty)" -o gx .

install:
	go install -ldflags "-X github.com/elentok/gx/cmd.version=$(shell git describe --tags --always --dirty)" .

# -timeout=5m: legitimate tests (incl. real-git/production-style) finish in a few seconds each
# locally, but CI runners' lower core counts give concurrency-sensitive tests (e.g. park/resume
# against a permit cap) more room to legitimately run long; 2m proved too tight there.
test:
	go test ./... -timeout=5m

# e2e runs the herdr e2e suite against a real, already-running herdr daemon
# (herdr must be on PATH). Tests skip themselves if either isn't available.
e2e:
	HERDR_ENV=1 go test ./e2e/... -timeout=2m

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
