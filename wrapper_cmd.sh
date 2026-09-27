#!/bin/sh
# Downloads the release binary for the detected platform and runs it.
#
#   curl -sSL https://github.com/castai/dbo-deployment-wizard/releases/latest/download/wrapper_cmd.sh | sh -s -- --api-secret=...
#
# goreleaser rewrites VERSION below when attaching this script to a release.
set -eu

REPO="castai/dbo-deployment-wizard"
VERSION="0.0.1"
BIN_NAME="dbo-deployment-wizard"

# --- detect platform ---
OS_RAW="$(uname -s)"
ARCH_RAW="$(uname -m)"

case "$OS_RAW" in
  Darwin) OS="darwin" ;;
  Linux)  OS="linux" ;;
  *) echo "Unsupported OS: $OS_RAW" >&2; exit 1 ;;
esac

case "$ARCH_RAW" in
  arm64|aarch64) ARCH="arm64" ;;
  x86_64)        ARCH="amd64" ;;
  *) echo "Unsupported arch: $ARCH_RAW" >&2; exit 1 ;;
esac

ASSET="${BIN_NAME}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ASSET}"

echo "Downloading: ${URL}"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

curl -sSL -o "${WORKDIR}/${ASSET}" "$URL"

tar -xzf "${WORKDIR}/${ASSET}" -C "$WORKDIR"

BIN_PATH="${WORKDIR}/${BIN_NAME}"
if [ ! -f "$BIN_PATH" ]; then
  # fall back: find the binary inside the extracted tree in case the
  # archive nests it in a subfolder
  BIN_PATH="$(find "$WORKDIR" -type f -name "$BIN_NAME" | head -n1)"
fi

if [ -z "$BIN_PATH" ] || [ ! -f "$BIN_PATH" ]; then
  echo "Could not locate '${BIN_NAME}' inside the extracted archive." >&2
  exit 1
fi

chmod +x "$BIN_PATH"

# Strip the macOS quarantine flag so Gatekeeper doesn't block execution.
# No-op (and harmless) on Linux, where this attribute doesn't exist.
xattr -d com.apple.quarantine "$BIN_PATH" 2>/dev/null || true

# execute with passed flags
"$BIN_PATH" "$@" < /dev/tty
