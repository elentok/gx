.PHONY: build install test test-docker-ubuntu run demos demo-seed

GO_VERSION := 1.25.0

DEMO_DIR   := web/demo
DEMO_WORK  := $(DEMO_DIR)/.work
DEMO_TAPES := $(DEMO_DIR)/tapes

build:
	go build -ldflags "-X github.com/elentok/gx/cmd.version=$(shell git describe --tags --always --dirty)" -o gx .

install:
	go install -ldflags "-X github.com/elentok/gx/cmd.version=$(shell git describe --tags --always --dirty)" .

test:
	go test ./...

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
# Builds gx, seeds the fixture, then renders every VHS tape. The .tape files
# arrive in a later task; until then this just (re)builds the fixture.
demos: build demo-seed
	@if ls $(DEMO_TAPES)/*.tape >/dev/null 2>&1; then \
		for tape in $(DEMO_TAPES)/*.tape; do \
			echo "vhs $$tape"; \
			PATH="$(CURDIR):$$PATH" vhs "$$tape"; \
		done; \
	else \
		echo "no tapes in $(DEMO_TAPES) yet — see beads main-3vg.3"; \
	fi
