#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
cache="$PWD/cache"
mkdir -p "$cache"/{tmp,gocache,gomod,gopath,config}
# Keep build tools and their subprocesses local to the checkout.
export TMPDIR="$cache/tmp" TEMP="$cache/tmp" TMP="$cache/tmp"
export GOCACHE="$cache/gocache" GOPATH="$cache/gopath" GOMODCACHE="$cache/gomod"
export XDG_CONFIG_HOME="$cache/config" TEST_TELEMETRY_DIR="$cache/telemetry"
export npm_config_cache="$cache/npm" npm_config_update_notifier=false
# Exclude cached Go sources from ./... when the caller runs tests afterwards.
[[ -f "$cache/go.mod" ]] || echo 'module local-build-cache' > "$cache/go.mod"
if [[ "${SKIP_NPM_CI:-0}" != 1 ]]; then (cd frontend && npm ci --no-fund --no-audit); fi
(cd frontend && npm run build)
go run github.com/tc-hib/go-winres@v0.3.3 make --in build/windows/winres.json --out resource
mkdir -p dist
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -trimpath -tags desktop,production,wv2runtime.error -ldflags '-s -w -H windowsgui' -o dist/DevEnvManager.exe .
