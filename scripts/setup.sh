#!/bin/bash
set -e

echo "╔══════════════════════════════════════════════════════════════════╗"
echo "║                    Continua Development Setup                     ║"
echo "╚══════════════════════════════════════════════════════════════════╝"

check_command() {
    if ! command -v "$1" &> /dev/null; then
        echo "❌ $1 is not installed. Please install it first."
        exit 1
    fi
    echo "✓ $1 found"
}

echo "Checking prerequisites..."
check_command go
check_command node
check_command pnpm
check_command docker
check_command uv

make setup

echo ""
echo "✅ Setup complete!"
echo ""
echo "Next steps:"
echo "  make dev          - Start database"
echo "  make dev-server   - Start Go backend"
echo "  make dev-web      - Start web UI"
