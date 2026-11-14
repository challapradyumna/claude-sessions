#!/bin/sh
# Install claude-sessions on macOS or Linux:
#   curl -fsSL https://raw.githubusercontent.com/challapradyumna/claude-sessions/main/install.sh | sh
# Options (environment variables):
#   CLAUDE_SESSIONS_INSTALL_DIR   where to put the binary (default ~/.local/bin)
#   CLAUDE_SESSIONS_VERSION       a release tag such as v0.1.0 (default: latest)
set -eu

REPO="challapradyumna/claude-sessions"
DIR="${CLAUDE_SESSIONS_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${CLAUDE_SESSIONS_VERSION:-latest}"
if [ "$VERSION" = latest ]; then BASE="https://github.com/$REPO/releases/latest/download"
else BASE="https://github.com/$REPO/releases/download/$VERSION"; fi
BASE="${CLAUDE_SESSIONS_BASE_URL:-$BASE}"   # for testing against a local build

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux)  os=linux ;;
  *) echo "Unsupported OS: $(uname -s). On Windows, use install.ps1." >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "Unsupported CPU: $(uname -m)" >&2; exit 1 ;;
esac
asset="claude-sessions_${os}_${arch}"

fetch() {  # fetch URL FILE
  if command -v curl >/dev/null 2>&1; then curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then wget -qO "$2" "$1"
  else echo "Need curl or wget." >&2; exit 1; fi
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "Downloading $asset ($VERSION)..."
fetch "$BASE/$asset" "$tmp/$asset"
fetch "$BASE/checksums.txt" "$tmp/checksums.txt"

expected=$(awk -v a="$asset" '$2 == a || $2 == "*"a {print $1}' "$tmp/checksums.txt")
if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$tmp/$asset" | awk '{print $1}')
else actual=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}'); fi
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
  echo "Checksum mismatch for $asset (expected '$expected', got '$actual'). Aborting." >&2
  exit 1
fi

mkdir -p "$DIR"
mv "$tmp/$asset" "$DIR/claude-sessions"
chmod +x "$DIR/claude-sessions"
echo "Installed $("$DIR/claude-sessions" version) to $DIR/claude-sessions"

case ":$PATH:" in
  *":$DIR:"*) ;;
  *) echo "Note: $DIR is not on your PATH. Add this to your shell profile:"
     echo "  export PATH=\"$DIR:\$PATH\"" ;;
esac
