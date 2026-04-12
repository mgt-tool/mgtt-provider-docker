#!/bin/bash
set -e
cd "$(dirname "$0")/.."
mkdir -p bin
go build -o bin/mgtt-provider-docker .
echo "✓ built bin/mgtt-provider-docker"
