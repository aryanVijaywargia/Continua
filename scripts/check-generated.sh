#!/bin/bash
set -e

make generate > /dev/null 2>&1

if [[ -n $(git status --porcelain) ]]; then
    echo "❌ Generated code is out of sync!"
    git status --porcelain
    exit 1
fi

echo "✓ Generated code is in sync"
