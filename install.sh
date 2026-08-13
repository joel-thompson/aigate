#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

echo "Building aigate..."
go build -o aigate ./cmd/aigate

echo "Installing to ${INSTALL_DIR}/aigate..."
if [ -w "${INSTALL_DIR}" ]; then
  cp aigate "${INSTALL_DIR}/aigate"
else
  sudo cp aigate "${INSTALL_DIR}/aigate"
fi
rm aigate

echo "Done. Run 'aigate --help' to get started."
