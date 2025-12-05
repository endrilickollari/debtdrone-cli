#!/bin/bash
set -e

# Configuration
OWNER="endrilickollari"
REPO="debtdrone-cli"
BINARY_NAME="debtdrone"

# 1. Detect OS & Architecture
OS="$(uname -s)"
ARCH="$(uname -m)"

# Map OS names to your build naming convention
case "$OS" in
    Linux)  PLATFORM="linux" ;;
    Darwin) PLATFORM="darwin" ;;
    *)
        echo "❌ Unsupported OS: $OS"
        echo "Please build from source: https://github.com/$OWNER/$REPO"
        exit 1
        ;;
esac

# Map Architecture names (x86_64 -> amd64, etc.)
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    arm64)   ARCH="arm64" ;;
    aarch64) ARCH="arm64" ;;
    *)
        echo "❌ Unsupported Architecture: $ARCH"
        exit 1
        ;;
esac

# 2. Construct the Download URL
# Note: This assumes you upload binaries named like 'debtdrone-linux-amd64' to Releases
ASSET_NAME="${BINARY_NAME}-${PLATFORM}-${ARCH}"
DOWNLOAD_URL="https://github.com/${OWNER}/${REPO}/releases/latest/download/${ASSET_NAME}"

echo "🔍 Detected platform: $OS ($ARCH)"
echo "⬇️  Downloading ${BINARY_NAME}..."

# 3. Download the binary (fails if asset not found)
if ! curl -fL -o "${BINARY_NAME}" "${DOWNLOAD_URL}"; then
    echo "❌ Download failed. Could not fetch: $DOWNLOAD_URL"
    echo "Check if a release exists for this version."
    exit 1
fi

# 4. Make it executable
chmod +x "${BINARY_NAME}"

# 5. Install to PATH (handle permissions)
INSTALL_DIR="/usr/local/bin"

echo "📦 Installing to $INSTALL_DIR..."

# Check if we have write access to /usr/local/bin, otherwise use sudo
if [ -w "$INSTALL_DIR" ]; then
    mv "${BINARY_NAME}" "$INSTALL_DIR/"
else
    echo "🔒 Sudo permission required to move binary to $INSTALL_DIR"
    sudo mv "${BINARY_NAME}" "$INSTALL_DIR/"
fi

# 6. Verify installation
echo "✅ Installation complete!"
echo "🚀 Run '$BINARY_NAME --help' to get started."
