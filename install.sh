#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"

echo "Building aigate ${VERSION}..."
go build -ldflags "-X main.version=${VERSION}" -o aigate ./cmd/aigate

echo "Installing to ${INSTALL_DIR}/aigate..."
if [ -w "${INSTALL_DIR}" ]; then
  cp aigate "${INSTALL_DIR}/aigate"
else
  sudo cp aigate "${INSTALL_DIR}/aigate"
fi
rm aigate

echo "Done. Run 'aigate --help' to get started."
