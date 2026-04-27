.PHONY: build install test test-docker-ubuntu run

GO_VERSION := 1.25.0

build:
	go build -ldflags "-X github.com/elentok/gx/cmd.version=$(shell git describe --tags --always --dirty)" -o gx .

install:
	go install .

test:
	go test ./...

test-docker-ubuntu:
	docker run --rm \
		-v $(CURDIR):/work \
		-w /work \
		-e GO_VERSION=$(GO_VERSION) \
		ubuntu:24.04 \
		bash ./scripts/run-tests-inside-docker-ubuntu.sh

run:
	go run .
