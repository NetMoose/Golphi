#!/bin/sh
# Build-only proof: no GUI runtime is invoked.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT=${OUT:-/tmp/golphi-mvp-build}
GOCACHE=${GOCACHE:-/tmp/golphi-gocache}
GOMODCACHE=${GOMODCACHE:-/tmp/golphi-gomodcache}
GOROOT=$(go env GOROOT)
BWRAP=$(command -v bwrap)
BUSYBOX=$(command -v busybox)
case "$OUT:$GOCACHE:$GOMODCACHE" in *[!a-zA-Z0-9_./:-]*) echo 'Use paths without whitespace or shell metacharacters' >&2; exit 1;; esac
for path in "$OUT" "$GOCACHE" "$GOMODCACHE"; do
    case "$path" in /*) ;; *) echo "Use absolute paths: $path" >&2; exit 1;; esac
done
mkdir -p "$OUT" "$GOCACHE"
test -d "$GOMODCACHE"
# The sandbox root contains only these mounts; /usr, /bin and /lib are absent.
run() {
    "$BWRAP" --unshare-user --unshare-pid --unshare-uts --unshare-ipc \
        --die-with-parent --ro-bind "$GOROOT" /go --ro-bind "$ROOT" /src \
        --ro-bind "$GOMODCACHE" /modules --bind "$GOCACHE" /cache \
        --bind "$OUT" /out --ro-bind "$BUSYBOX" /proof/busybox \
        --tmpfs /tmp --proc /proc --dev /dev --chdir /src --clearenv \
        --setenv PATH /go/bin --setenv GOROOT /go --setenv HOME /tmp \
        --setenv GOCACHE /cache --setenv GOMODCACHE /modules \
        --setenv GOPROXY off --setenv GOSUMDB off --setenv GOTOOLCHAIN local \
        --setenv GOAMD64 v1 --setenv CGO_ENABLED 0 --setenv GOOS "$1" --setenv GOARCH amd64 \
        /proof/busybox sh -eu -c "$2"
}
build() {
    echo 'Minimal mount namespace: read-only project, Go toolchain, module cache; writable build cache/output.'
    run linux '
        test ! -e /usr && test ! -e /bin && test ! -e /lib && test ! -e /include
        for tool in cc gcc clang g++ pkg-config; do
            if command -v "$tool"; then exit 1; fi
        done
        echo "PASS: /usr /bin /lib /include absent; no C compiler or pkg-config in PATH"
        test "$(/go/bin/go version)" = "go version go1.26.5 linux/amd64"
        /go/bin/go version
        /go/bin/go env GOOS GOARCH CGO_ENABLED GOTOOLCHAIN GOPROXY GOSUMDB
        /go/bin/go list -m all
    '
    for target in linux windows; do
        suffix=
        test "$target" != windows || suffix=.exe
        echo "GOOS=$target GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/hello-$target-amd64$suffix ./examples/hello"
        run "$target" "/go/bin/go build -trimpath -buildvcs=false -o /out/hello-$target-amd64$suffix ./examples/hello
/go/bin/go build -trimpath -buildvcs=false -o /out/hello-$target-amd64-repeat$suffix ./examples/hello
/proof/busybox cmp /out/hello-$target-amd64$suffix /out/hello-$target-amd64-repeat$suffix
/proof/busybox rm /out/hello-$target-amd64-repeat$suffix
/go/bin/go version -m /out/hello-$target-amd64$suffix"
        echo "PASS: $target repeated build is byte-identical"
    done
    (cd "$OUT" && sha256sum hello-linux-amd64 hello-windows-amd64.exe > SHA256SUMS)
    cat "$OUT/SHA256SUMS"
}
if [ "${GOLPHI_BUILD_INNER:-}" = 1 ]; then
    build
else
    status=0
    GOLPHI_BUILD_INNER=1 "$0" > "$OUT/build.log" 2>&1 || status=$?
    cat "$OUT/build.log"
    exit "$status"
fi
