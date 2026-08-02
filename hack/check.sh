#!/bin/sh
set -eu

toolchain='golang:1.24.6-bookworm@sha256:ab1d1823abb55a9504d2e3e003b75b36dbeb1cbcc4c92593d85a84ee46becc6c'
project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

test -s "$project_root/cmd/unapid/main.go"
grep -Fq 'Copyright 2026 Relmio contributors' "$project_root/NOTICE"
grep -Fq 'https://github.com/Demonbane18/relmio' "$project_root/UPSTREAM.md"

exec docker run --rm \
  --volume "$project_root:/src" \
  --workdir /src \
  "$toolchain" \
  sh -eu -c '
    test -z "$(gofmt -l cmd internal)"
    go vet ./...
    go test ./...
    CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /tmp/unapid-check ./cmd/unapid
  '
