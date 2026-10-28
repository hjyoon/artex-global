#!/usr/bin/env bash
# Start the local ChatGPT-authenticated Codex adapter and the ARTEX Compose app.
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_dir"
python3 runtime/codex_gateway.py --ensure-running
docker compose up -d
