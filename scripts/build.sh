#!/bin/sh
# Cross-compile release binaries into dist/ with a checksums file.
#   scripts/build.sh 0.1.0
set -eu
VERSION=${1:?usage: scripts/build.sh VERSION}
cd "$(dirname "$0")/.."
rm -rf dist && mkdir dist
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  os=${target%/*}; arch=${target#*/}
  ext=""; [ "$os" = windows ] && ext=".exe"
  out="dist/claude-sessions_${os}_${arch}${ext}"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$out" .
  echo "built $out"
done
cd dist
if command -v sha256sum >/dev/null 2>&1; then sha256sum claude-sessions_* > checksums.txt
else shasum -a 256 claude-sessions_* > checksums.txt; fi
cat checksums.txt
