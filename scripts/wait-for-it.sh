#!/bin/bash
set -e

HOST="$1"
PORT="$2"
MAX_ATTEMPTS="${3:-30}"

if [ -z "$HOST" ] || [ -z "$PORT" ]; then
    echo "Usage: wait-for-it.sh <host> <port> [max_attempts]"
    exit 1
fi

echo "Waiting for $HOST:$PORT..."

attempt=0
until nc -z "$HOST" "$PORT" 2>/dev/null; do
    attempt=$((attempt + 1))
    if [ $attempt -ge $MAX_ATTEMPTS ]; then
        echo "Service $HOST:$PORT not ready after $MAX_ATTEMPTS attempts."
        exit 1
    fi
    sleep 1
done

echo "$HOST:$PORT is available!"
