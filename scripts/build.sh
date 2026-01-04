#!/bin/bash
set -e

make generate
make build

echo "✅ Build complete: bin/continua"
