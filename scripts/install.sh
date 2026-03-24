#!/usr/bin/env sh
# observr installer
# Usage: curl -sSL https://raw.githubusercontent.com/your-org/observr/main/scripts/install.sh | sh

set -e

REPO="your-org/observr"
BIN_NAME="observrd"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# ── Detect OS / arch ──────────────────────────────────────────────────────
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *)
    echo "Unsupported OS: $OS"
    echo "Build from source: https://github.com/$REPO"
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64 | amd64) arch="amd64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    echo "Build from source: https://github.com/$REPO"
    exit 1
    ;;
esac

PLATFORM="${os}-${arch}"

# ── Resolve latest version ────────────────────────────────────────────────
if [ -z "$VERSION" ]; then
  VERSION="$(curl -sSf "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' | sed 's/.*"tag_name": *"\(.*\)".*/\1/')"
fi

if [ -z "$VERSION" ]; then
  echo "Could not determine latest version. Set VERSION env var to override."
  exit 1
fi

# ── Download ──────────────────────────────────────────────────────────────
FILENAME="${BIN_NAME}-${PLATFORM}"
URL="https://github.com/$REPO/releases/download/$VERSION/$FILENAME"
TMP="$(mktemp)"

echo "Installing observrd $VERSION for $PLATFORM..."
curl -sSfL "$URL" -o "$TMP"
chmod +x "$TMP"

# ── Install ───────────────────────────────────────────────────────────────
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP" "$INSTALL_DIR/$BIN_NAME"
else
  sudo mv "$TMP" "$INSTALL_DIR/$BIN_NAME"
fi

echo "observrd installed to $INSTALL_DIR/$BIN_NAME"
echo ""
echo "  observrd --port 7676   # start collector + dashboard"
echo "  observrd query --help  # query events from CLI"
echo ""
echo "Python SDK:"
echo "  pip install observr"
