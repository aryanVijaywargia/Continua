#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Pinned versions (must match mise.toml)
BUF_VERSION="1.32.0"

echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}        Continua Development Environment Setup              ${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo ""

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

check_command() {
    if command -v $1 &> /dev/null; then
        echo -e "  ${GREEN}✓${NC} $1 found"
        return 0
    else
        echo -e "  ${RED}✗${NC} $1 not found"
        return 1
    fi
}

MISSING=0
check_command "go" || MISSING=1
check_command "node" || MISSING=1
check_command "docker" || MISSING=1

# Check pnpm (install via corepack if missing)
if ! command -v pnpm &> /dev/null; then
    echo -e "  ${YELLOW}!${NC} pnpm not found, installing via corepack..."
    corepack enable
    corepack prepare pnpm@9.1.0 --activate
fi
check_command "pnpm" || MISSING=1

# Check uv (install if missing)
if ! command -v uv &> /dev/null; then
    echo -e "  ${YELLOW}!${NC} uv not found, installing..."
    curl -LsSf https://astral.sh/uv/install.sh | sh
    export PATH="$HOME/.cargo/bin:$PATH"
fi
check_command "uv" || MISSING=1

# Check buf (install pinned version if missing or wrong version)
if ! command -v buf &> /dev/null; then
    echo -e "  ${YELLOW}!${NC} buf not found, installing v${BUF_VERSION}..."
    go install github.com/bufbuild/buf/cmd/buf@v${BUF_VERSION}
else
    CURRENT_BUF=$(buf --version 2>/dev/null || echo "0.0.0")
    if [ "$CURRENT_BUF" != "$BUF_VERSION" ]; then
        echo -e "  ${YELLOW}!${NC} buf version mismatch (have $CURRENT_BUF, want $BUF_VERSION), updating..."
        go install github.com/bufbuild/buf/cmd/buf@v${BUF_VERSION}
    fi
fi
check_command "buf" || MISSING=1

if [ $MISSING -eq 1 ]; then
    echo ""
    echo -e "${RED}Please install missing prerequisites and try again.${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}All prerequisites found!${NC}"
echo ""

# Enable corepack for consistent pnpm version
echo -e "${YELLOW}Enabling corepack...${NC}"
corepack enable
echo -e "${GREEN}✓ Corepack enabled${NC}"
echo ""

# Install Go dependencies
echo -e "${YELLOW}Installing Go dependencies...${NC}"
go work sync
cd server && go mod download && cd ..
cd packages/proto-go && go mod download && cd ../..
echo -e "${GREEN}✓ Go dependencies installed${NC}"
echo ""

# Install Node dependencies
echo -e "${YELLOW}Installing Node dependencies...${NC}"
pnpm install
echo -e "${GREEN}✓ Node dependencies installed${NC}"
echo ""

# Install Python SDK dependencies
echo -e "${YELLOW}Setting up Python SDK...${NC}"
cd sdks/python && uv sync && cd ../..
echo -e "${GREEN}✓ Python SDK ready${NC}"
echo ""

# Generate proto code
echo -e "${YELLOW}Generating protobuf code...${NC}"
cd proto && buf generate && cd ..
echo -e "${GREEN}✓ Proto code generated${NC}"
echo ""

# Build packages in order
echo -e "${YELLOW}Building packages...${NC}"
pnpm --filter @continua/api-client build
pnpm --filter @continua/sdk build
cd server && go build ./... && cd ..
echo -e "${GREEN}✓ All packages built${NC}"
echo ""

# Summary
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}        Setup Complete!                                     ${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo ""
echo "Quick commands:"
echo -e "  ${CYAN}make dev${NC}           - Start all services for development"
echo -e "  ${CYAN}make test${NC}          - Run all tests"
echo -e "  ${CYAN}make proto${NC}         - Regenerate proto code"
echo -e "  ${CYAN}make server-run${NC}    - Run server only"
echo -e "  ${CYAN}make web-dev${NC}       - Run web UI only"
echo ""
echo -e "Start developing: ${GREEN}make dev${NC}"
echo ""
