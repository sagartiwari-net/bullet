#!/bin/bash
set -e
cd "$(dirname "$0")"

mkdir -p results configs wordlists

echo "Building..."
go build -o bin/checker .
go build -o bin/demo-server ./demo-server

echo ""
echo "Start demo server (optional, for testing):"
echo "  ./bin/demo-server"
echo ""
echo "Start checker dashboard:"
echo "  ./bin/checker"
echo ""
echo "Or with Docker:"
echo "  docker compose up -d --build"
