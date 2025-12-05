#!/bin/bash
set -e

# Configuration
OWNER="endrilickollari"
REPO="debtdrone-cli"
BINARY_NAME="debtdrone"

# 1. Detect OS & Architecture
OS="$(uname -s)"
ARCH="$(uname -m)"

# Map OS to GoReleaser format (Title Case: Darwin, Linux)
case "$OS" in
    Linux)  PLATFORM="Linux" ;;
    Darwin) PLATFORM="Darwin" ;;
    *)
        echo "❌ Unsupported OS: $OS"
        exit 1
        ;;
esac

# Map Architecture to GoReleaser format (x86_64, arm64)
case "$ARCH" in
    x86_64|amd64) ARCH="x86_64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)
        echo "❌ Unsupported Architecture: $ARCH"
        exit 1
        ;;
esac

# 2. Construct the Download URL (Matching your actual release filenames)
# Format: debtdrone_Darwin_arm64.tar.gz
ASSET_NAME="${BINARY_NAME}_${PLATFORM}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${OWNER}/${REPO}/releases/latest/download/${ASSET_NAME}"

echo "🔍 Detected platform: $OS ($ARCH)"
echo "⬇️  Downloading $ASSET_NAME..."

# Create a temp directory for extraction
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

# 3. Download the archive
if ! curl -fL -o "$TMP_DIR/$ASSET_NAME" "$DOWNLOAD_URL"; then
    echo "❌ Download failed. Could not fetch: $DOWNLOAD_URL"
    echo "Check if a release exists for version: latest"
    exit 1
fi

# 4. Extract the binary
echo "📦 Extracting..."
tar -xzf "$TMP_DIR/$ASSET_NAME" -C "$TMP_DIR"

# 5. Install to PATH
INSTALL_DIR="/usr/local/bin"
TARGET_PATH="$INSTALL_DIR/$BINARY_NAME"

echo "🚀 Installing to $INSTALL_DIR..."

# Check permissions
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/$BINARY_NAME" "$TARGET_PATH"
else
    echo "🔒 Sudo permission required to move binary to $INSTALL_DIR"
    sudo mv "$TMP_DIR/$BINARY_NAME" "$TARGET_PATH"
fi

# Make executable just in case
chmod +x "$TARGET_PATH"

# 6. Verify
echo "✅ Installation complete!"
echo "👉 Run '$BINARY_NAME --help' to get started."
