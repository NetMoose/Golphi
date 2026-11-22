#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
export CGO_ENABLED=0 GOWORK=off GOFLAGS=-mod=readonly
GOOS=$(go env GOHOSTOS)
GOARCH=$(go env GOHOSTARCH)
export GOOS GOARCH

for example in hello dynamic-tree; do
    app="$root/examples/$example"
    go -C "$root/tools/winres" tool go-winres make \
        --in "$app/winres.json" --out "$app/rsrc" --arch amd64
done
