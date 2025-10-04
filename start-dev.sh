#!/bin/bash

# DataBridge Development Startup Script
# Starts backend with INFO level logging (not DEBUG)

set -e

echo "🚀 Starting DataBridge (Development Mode)"
echo ""

# Check if .env exists, create from example if not
if [ ! -f .env ]; then
    echo "Creating .env from .env.example..."
    cp .env.example .env
    echo "✓ .env created (edit as needed)"
    echo ""
fi

# Source environment variables
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# Set default log level if not specified
LOG_LEVEL=${LOG_LEVEL:-info}

# Build if needed
if [ ! -f ./build/databridge ]; then
    echo "Building DataBridge..."
    make build
    echo ""
fi

echo "Starting DataBridge backend..."
echo "  Log Level: $LOG_LEVEL"
echo "  API Port: ${API_PORT:-8080}"
echo "  Data Dir: ${DATA_DIR:-./data}"
echo ""
echo "Backend will be available at: http://localhost:${API_PORT:-8080}"
echo "Press Ctrl+C to stop"
echo ""

# Start with specified log level
./build/databridge --log-level "$LOG_LEVEL" "$@"
