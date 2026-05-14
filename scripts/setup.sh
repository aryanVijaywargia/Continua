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

echo "Checking prerequisite tools..."
check_command go
check_command node
check_command pnpm
check_command docker
check_command uv

echo ""
echo "This script verifies the required toolchain and installs repository dependencies."
echo "It does not install Go, Node.js, Docker, Python, pnpm, or uv for you."

GOBIN_DIR="$(go env GOPATH)/bin"
mkdir -p "$GOBIN_DIR"

install_go_tool() {
    local bin_name="$1"
    local pkg="$2"
    if [ -x "$GOBIN_DIR/$bin_name" ] || command -v "$bin_name" &> /dev/null; then
        echo "✓ $bin_name found"
        return
    fi
    echo "→ Installing $bin_name from $pkg"
    GOBIN="$GOBIN_DIR" go install "$pkg"
    echo "✓ $bin_name installed to $GOBIN_DIR"
}

install_golangci_lint() {
    if command -v golangci-lint &> /dev/null; then
        echo "✓ golangci-lint found"
        return
    fi
    echo "→ Installing golangci-lint via official installer"
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
        | sh -s -- -b "$GOBIN_DIR"
    echo "✓ golangci-lint installed to $GOBIN_DIR"
}

echo ""
echo "Installing Go developer tools..."
install_go_tool air         github.com/air-verse/air@latest
install_go_tool sqlc        github.com/sqlc-dev/sqlc/cmd/sqlc@latest
install_go_tool oapi-codegen github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
install_golangci_lint

case ":$PATH:" in
    *":$GOBIN_DIR:"*) ;;
    *)
        echo ""
        echo "⚠️  $GOBIN_DIR is not on your PATH."
        echo "   Add this to your shell profile so installed tools are discoverable:"
        echo "     export PATH=\"$GOBIN_DIR:\$PATH\""
        ;;
esac

echo ""
make setup

echo ""
echo "✅ Setup complete!"
echo ""
echo "Next steps:"
echo "  make dev          - Start database"
echo "  make dev-server   - Start Go backend"
echo "  make dev-web      - Start web UI"
