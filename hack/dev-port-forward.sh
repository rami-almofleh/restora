#!/usr/bin/env bash
# Opens the Restora web interface of the local dev cluster at http://localhost:8080 (or the port you pass).
set -euo pipefail

PORT="${1:-8080}"
echo "Restora is available at http://localhost:$PORT  (press Ctrl+C to stop)"
exec kubectl --context restora-dev --namespace restora-system port-forward service/restora-api "$PORT:8080"
