#!/bin/sh
set -eu

toolchain='golang:1.24.6-bookworm@sha256:ab1d1823abb55a9504d2e3e003b75b36dbeb1cbcc4c92593d85a84ee46becc6c'
project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output_directory="$project_root/dist"

install -d -m 0755 "$output_directory"

docker run --rm \
  --volume "$project_root:/src" \
  --workdir /src \
  "$toolchain" \
  sh -eu -c '
    gofmt -w cmd internal
    go vet ./...
    go test ./...
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="-s -w" -o dist/unapid-linux-amd64 ./cmd/unapid
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -buildvcs=false -ldflags="-s -w" -o dist/unapid-linux-arm64 ./cmd/unapid
  '

amd64_sha=$(sha256sum "$output_directory/unapid-linux-amd64" | awk '{print $1}')
arm64_sha=$(sha256sum "$output_directory/unapid-linux-arm64" | awk '{print $1}')

sed \
  -e "s/__AMD64_SHA256__/$amd64_sha/g" \
  -e "s/__ARM64_SHA256__/$arm64_sha/g" \
  "$project_root/packaging/install.sh.in" > "$output_directory/install.sh"
chmod 0755 \
  "$output_directory/install.sh" \
  "$output_directory/unapid-linux-amd64" \
  "$output_directory/unapid-linux-arm64"

if grep -q '__[A-Z0-9_]*__' "$output_directory/install.sh"; then
  printf '%s\n' 'Installer generation left an unresolved placeholder.' >&2
  exit 1
fi

printf '%s\n' "$output_directory/install.sh"
printf '%s\n' "$output_directory/unapid-linux-amd64"
printf '%s\n' "$output_directory/unapid-linux-arm64"
