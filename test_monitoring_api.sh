#!/bin/bash

# Test script for DataBridge Monitoring API
# Usage: ./test_monitoring_api.sh [host:port]

HOST="${1:-localhost:8080}"
BASE_URL="http://$HOST"

echo "Testing DataBridge Monitoring API at $BASE_URL"
echo "================================================"
echo

# Test health endpoint
echo "1. Testing /health endpoint..."
curl -s "$BASE_URL/health"
echo -e "\n"

# Test system status
echo "2. Testing /api/system/status endpoint..."
curl -s "$BASE_URL/api/system/status" | jq '.'
echo

# Test system health
echo "3. Testing /api/system/health endpoint..."
curl -s "$BASE_URL/api/system/health" | jq '.'
echo

# Test system metrics
echo "4. Testing /api/system/metrics endpoint..."
curl -s "$BASE_URL/api/system/metrics" | jq '.throughput, .memory, .cpu'
echo

# Test system stats
echo "5. Testing /api/system/stats endpoint..."
curl -s "$BASE_URL/api/system/stats" | jq '{totalFlowFilesProcessed, activeProcessors, totalProcessors, totalConnections}'
echo

# Test processor metrics
echo "6. Testing /api/monitoring/processors endpoint..."
curl -s "$BASE_URL/api/monitoring/processors" | jq 'map({id, name, type, state, tasksCompleted})'
echo

# Test connection metrics
echo "7. Testing /api/monitoring/connections endpoint..."
curl -s "$BASE_URL/api/monitoring/connections" | jq 'map({id, name, sourceName, destinationName, queueDepth})'
echo

# Test queue metrics
echo "8. Testing /api/monitoring/queues endpoint..."
curl -s "$BASE_URL/api/monitoring/queues" | jq '{totalQueued, totalCapacity, percentFull}'
echo

echo "================================================"
echo "API Test Complete!"
echo
echo "To test SSE streaming, open a browser and navigate to:"
echo "  $BASE_URL/api/monitoring/stream"
echo
echo "Or use curl:"
echo "  curl -N $BASE_URL/api/monitoring/stream"
