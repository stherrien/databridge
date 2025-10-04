#!/bin/bash

# DataBridge Kill All Script
# Stops all running DataBridge backend and frontend processes

set -e

echo "🛑 Stopping all DataBridge processes..."
echo ""

# Kill backend processes
echo "Stopping backend (databridge)..."
if pkill -9 databridge 2>/dev/null; then
    echo "✓ Backend processes killed"
else
    echo "✓ No backend processes running"
fi

# Kill frontend processes (Next.js)
echo "Stopping frontend (Next.js)..."
if pkill -9 -f "next dev" 2>/dev/null; then
    echo "✓ Frontend dev server killed"
else
    echo "✓ No frontend dev server running"
fi

# Kill any node processes related to the frontend
if pkill -9 -f "node.*frontend" 2>/dev/null; then
    echo "✓ Frontend node processes killed"
fi

# Check for any remaining processes on ports 8080 and 3000
echo ""
echo "Checking ports..."

# Port 8080 (backend)
BACKEND_PID=$(lsof -ti:8080 2>/dev/null || true)
if [ -n "$BACKEND_PID" ]; then
    echo "Killing process on port 8080: $BACKEND_PID"
    kill -9 $BACKEND_PID 2>/dev/null || true
    echo "✓ Port 8080 cleared"
else
    echo "✓ Port 8080 is free"
fi

# Port 3000 (frontend)
FRONTEND_PID=$(lsof -ti:3000 2>/dev/null || true)
if [ -n "$FRONTEND_PID" ]; then
    echo "Killing process on port 3000: $FRONTEND_PID"
    kill -9 $FRONTEND_PID 2>/dev/null || true
    echo "✓ Port 3000 cleared"
else
    echo "✓ Port 3000 is free"
fi

echo ""
echo "✅ All DataBridge processes stopped"
echo ""
echo "You can now start fresh with:"
echo "  Backend:  ./build/databridge"
echo "  Frontend: cd frontend && npm run dev"
