#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "${SKIP_NPM_CI:-0}" != 1 ]]; then (cd frontend && npm ci --no-fund --no-audit); fi
(cd frontend && npm run build)
go run github.com/tc-hib/go-winres@v0.3.3 make --in build/windows/winres.json --out resource
mkdir -p dist
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -trimpath -tags desktop,production,wv2runtime.error -ldflags '-s -w -H windowsgui' -o dist/DevEnvManager.exe .
