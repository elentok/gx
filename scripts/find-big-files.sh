#!/usr/bin/env bash

set -euo pipefail

for file in **/*.go; do
  wc -l "$file"
done | awk '$1 >= 400' | sort -n
