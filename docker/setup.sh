#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "Starting Milvus stack..."
docker compose up -d

echo "Waiting for Milvus to be healthy..."
for i in $(seq 1 60); do
    if curl -sf http://localhost:9091/healthz > /dev/null 2>&1; then
        echo "Milvus is healthy."
        break
    fi
    if [ "$i" -eq 60 ]; then
        echo "ERROR: Milvus did not become healthy within 60s"
        exit 1
    fi
    sleep 1
done

echo "Creating hook-events collection..."
curl -sf -X POST "http://localhost:19530/v2/vectordb/collections/create" \
    -H "Content-Type: application/json" \
    -d '{
        "collectionName": "hook_events",
        "dimension": 384,
        "metricType": "COSINE",
        "description": "Hook event embeddings for semantic search"
    }' && echo ""

echo "Setup complete."
echo "  Milvus API:    http://localhost:19530"
echo "  Milvus WebUI:  http://localhost:9091"
echo "  MinIO Console: http://localhost:9001"
