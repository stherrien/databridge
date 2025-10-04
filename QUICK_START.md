# DataBridge Quick Start Guide

## Starting DataBridge

### Option 1: Start Both Backend and Frontend (Recommended)
```bash
make dev
```
This will:
- Check and install frontend dependencies if needed
- Start backend on http://localhost:8080
- Start frontend on http://localhost:3000
- Run both processes with live reload

### Option 2: Start Individually

**Backend Only:**
```bash
# Build and run
make run

# OR run in development mode with debug logging
make run-dev

# OR run from source
go run cmd/databridge/main.go
```

**Frontend Only:**
```bash
# First time setup
cd frontend && npm install

# Start dev server
npm run dev

# OR use make
make frontend-dev
```

---

## Stopping DataBridge

### Kill All Processes (Recommended)
```bash
# Using the script directly
./kill-all.sh

# OR using make
make kill
```

This will:
- ✓ Stop all backend processes
- ✓ Stop all frontend/Next.js processes
- ✓ Kill any processes on port 8080
- ✓ Kill any processes on port 3000
- ✓ Clean up any orphaned processes

### Manual Shutdown
If you started with `make dev`, press `Ctrl+C` to stop both servers.

---

## Verifying Status

### Check if services are running:
```bash
# Check backend (should return JSON)
curl http://localhost:8080/api/flows

# Check frontend (should return HTML)
curl -I http://localhost:3000

# Check ports
lsof -i :8080  # Backend
lsof -i :3000  # Frontend
```

### Check logs:
```bash
# Backend logs (if running in background)
tail -f /tmp/databridge.log

# Frontend logs (if running in background)
tail -f /tmp/frontend.log
```

---

## Common Commands

| Command | Description |
|---------|-------------|
| `make dev` | Start both backend and frontend |
| `make kill` | Stop all DataBridge processes |
| `make build` | Build backend binary |
| `make test` | Run all tests |
| `make lint` | Lint code |
| `make clean` | Clean build artifacts |
| `make help` | Show all available commands |

---

## Troubleshooting

### Port Already in Use
```bash
# Kill all processes and restart
make kill
make dev
```

### Frontend Won't Start
```bash
# Reinstall dependencies
cd frontend
rm -rf node_modules .next
npm install
npm run dev
```

### Backend Won't Start
```bash
# Check if port is in use
lsof -i :8080

# Remove lock files
rm -rf data/flowfiles/LOCK

# Rebuild
make clean build run
```

### Database Issues
```bash
# Reset databases
make db-reset

# OR manually
rm -rf data/flowfiles
```

---

## Development Workflow

### 1. Initial Setup
```bash
# Clone repo
git clone <repo-url>
cd databridge

# Setup environment
make setup
```

### 2. Daily Development
```bash
# Start servers
make dev

# In another terminal, make changes...
# Backend auto-reloads with air (if configured)
# Frontend auto-reloads with Next.js

# Stop when done
make kill
```

### 3. Before Committing
```bash
# Run pre-commit checks
make pre-commit

# This runs:
# - Code formatting
# - Linting
# - Vetting
# - Unit tests
```

---

## Quick Testing

### Test the full stack:
```bash
# 1. Start services
make dev

# 2. In another terminal, test API
curl http://localhost:8080/api/flows

# 3. Open browser
open http://localhost:3000

# 4. Stop when done
make kill
```

---

## Environment Variables

Create a `.env` file in the root directory:

```bash
# Backend
DB_PATH=./data/flowfiles
API_PORT=8080
LOG_LEVEL=info

# Frontend (in frontend/.env.local)
NEXT_PUBLIC_API_URL=http://localhost:8080
```

---

## Next Steps

- Read [FRONTEND_TEST_PLAN.md](./FRONTEND_TEST_PLAN.md) for testing guidelines
- Read [FRONTEND_TEST_RESULTS.md](./FRONTEND_TEST_RESULTS.md) for current status
- Check [README.md](./README.md) for full documentation
- See `make help` for all available commands
