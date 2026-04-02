#!/usr/bin/env bash
#
# This script is designed to run inside a docker container using the `make
# test-docker-ubuntu` command

set -euo pipefail

GO_VERSION="${GO_VERSION:-1.25.0}"

case "$(uname -m)" in
	x86_64) GO_ARCH=amd64 ;;
	aarch64|arm64) GO_ARCH=arm64 ;;
	*) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y bash ca-certificates curl git git-delta
curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" | tar -C /usr/local -xz

PATH=/usr/local/go/bin:$PATH go test ./...
