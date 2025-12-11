#!/bin/bash
set -e

OWNER="endrilickollari"
REPO="debtdrone-cli"
BINARY_NAME="debtdrone"

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Linux)  OS_KEY="Linux" ;;
    Darwin) OS_KEY="Darwin" ;;
    *)      echo "❌ Unsupported OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
    x86_64|amd64) ARCH_KEY="x86_64" ;;
    arm64|aarch64) ARCH_KEY="arm64" ;;
    *)            echo "❌ Unsupported Architecture: $ARCH"; exit 1 ;;
esac

echo "🔍 Looking for ${OS_KEY} ${ARCH_KEY} binary..."

RELEASE_DATA=$(curl -s "https://api.github.com/repos/${OWNER}/${REPO}/releases")

DOWNLOAD_URL=$(echo "$RELEASE_DATA" | grep -o "https://.*release.*${OS_KEY}_${ARCH_KEY}.tar.gz" | head -1)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "❌ Could not find a release asset for ${OS_KEY}_${ARCH_KEY}"
    echo "Available assets might not match the naming convention."
    exit 1
fi

echo "⬇️  Downloading from: $DOWNLOAD_URL"

TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

curl -fL -o "$TMP_DIR/debtdrone.tar.gz" "$DOWNLOAD_URL"
tar -xzf "$TMP_DIR/debtdrone.tar.gz" -C "$TMP_DIR"

INSTALL_DIR="/usr/local/bin"
echo "🚀 Installing to $INSTALL_DIR..."

if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/"
else
    echo "🔒 Sudo permission required..."
    sudo mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/"
fi

chmod +x "$INSTALL_DIR/$BINARY_NAME"

echo "✅ Installation complete! Run '$BINARY_NAME --help' to start."
