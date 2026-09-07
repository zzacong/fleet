#!/bin/sh
# install.sh — install the fleet binary from GitHub Releases.
#
# Usage:
#   sh install.sh [VERSION]
#   VERSION=0.1.0 INSTALL_DIR=/usr/local/bin sh install.sh
#
# VERSION defaults to "latest" (leading "v" optional: 0.1.0 == v0.1.0).
# INSTALL_DIR defaults to ~/.local/bin. This script never edits shell rc
# files; if the install dir is not on PATH it prints an export hint instead.
set -eu

REPO="${REPO:-zzacong/fleet}"
VERSION="${VERSION:-${1:-latest}}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

fail() {
  echo "fleet install: $1" >&2
  exit 1
}

case "$VERSION" in
  -h | --help | help)
    echo "usage: sh install.sh [VERSION]  (default: latest)"
    echo "env: VERSION, INSTALL_DIR (default ~/.local/bin), REPO"
    exit 0
    ;;
esac

os_name="$(uname -s)"
case "$os_name" in
  Darwin) GOOS="darwin" ;;
  Linux) GOOS="linux" ;;
  *) fail "unsupported OS '$os_name' (supported: macOS, Linux)" ;;
esac

arch_name="$(uname -m)"
case "$arch_name" in
  arm64 | aarch64) GOARCH="arm64" ;;
  x86_64 | amd64) GOARCH="amd64" ;;
  *) fail "unsupported architecture '$arch_name' (supported: arm64, x86_64)" ;;
esac

# Must match .goreleaser.yaml archives name_template
# ("{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}", tar.gz): GoReleaser renders
# Os lowercase (darwin/linux) and Arch in Go spelling (amd64/arm64), e.g.
# fleet_darwin_arm64.tar.gz — verified via `goreleaser release --snapshot`.
ASSET="fleet_${GOOS}_${GOARCH}.tar.gz"

if [ "$VERSION" = "latest" ]; then
  BASE="https://github.com/${REPO}/releases/latest/download"
  LABEL="latest"
else
  case "$VERSION" in
    v*) TAG="$VERSION" ;;
    *) TAG="v$VERSION" ;;
  esac
  BASE="https://github.com/${REPO}/releases/download/${TAG}"
  LABEL="$TAG"
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

download() {
  # $1 = url, $2 = destination
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$2" "$1" || fail "cannot download $1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1" || fail "cannot download $1"
  else
    fail "need curl or wget to download releases"
  fi
}

echo "fleet install: fetching $LABEL for ${GOOS}/${GOARCH}"
download "$BASE/$ASSET" "$TMPDIR/$ASSET"
download "$BASE/checksums.txt" "$TMPDIR/checksums.txt"

EXPECTED="$(awk -v a="$ASSET" '$2 == a { print $1 }' "$TMPDIR/checksums.txt")"
[ -n "$EXPECTED" ] || fail "$ASSET not found in checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum "$TMPDIR/$ASSET" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL="$(shasum -a 256 "$TMPDIR/$ASSET" | awk '{ print $1 }')"
else
  fail "need sha256sum or shasum to verify the download"
fi

[ "$EXPECTED" = "$ACTUAL" ] || fail "SHA-256 mismatch for $ASSET (download corrupt?)"
echo "fleet install: checksum OK"

tar -xzf "$TMPDIR/$ASSET" -C "$TMPDIR"
[ -f "$TMPDIR/fleet" ] || fail "archive $ASSET contains no fleet binary"
mkdir -p "$INSTALL_DIR"
cp "$TMPDIR/fleet" "$INSTALL_DIR/fleet"
chmod 755 "$INSTALL_DIR/fleet"
echo "fleet install: installed to $INSTALL_DIR/fleet"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo "fleet install: add to PATH with:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac
