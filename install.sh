#!/bin/bash
set -e

INSTALL_DIR="/usr/local/bin"

# macOS: ~/Library/Application Support/Mozilla/NativeMessagingHosts
# Linux: ~/.mozilla/native-messaging-hosts
if [ "$(uname)" = "Darwin" ]; then
  MANIFEST_DIR="$HOME/Library/Application Support/Mozilla/NativeMessagingHosts"
else
  MANIFEST_DIR="$HOME/.mozilla/native-messaging-hosts"
fi
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Building foxstream-bridge..."
cd "$SCRIPT_DIR"
go build -o foxstream-bridge .

echo "Installing binary to $INSTALL_DIR..."
sudo cp foxstream-bridge "$INSTALL_DIR/"
sudo chmod +x "$INSTALL_DIR/foxstream-bridge"

echo "Registering native messaging manifest..."
mkdir -p "$MANIFEST_DIR"
cat > "$MANIFEST_DIR/foxstream_bridge.json" << EOF
{
  "name": "foxstream_bridge",
  "description": "FoxStream video download bridge",
  "path": "$INSTALL_DIR/foxstream-bridge",
  "type": "stdio",
  "allowed_extensions": ["foxstream@extension"]
}
EOF

echo "FoxStream Bridge installed successfully."
echo "Binary: $INSTALL_DIR/foxstream-bridge"
echo "Manifest: $MANIFEST_DIR/foxstream_bridge.json"
