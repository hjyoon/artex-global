#!/usr/bin/env bash
# Start ARTEX with the device-login UI and a dedicated Codex auth store.
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_dir"
python3 - <<'PY'
from pathlib import Path
import os
import secrets

path = Path('.env')
if not path.exists():
    raise SystemExit('Create .env from .env.example and set POSTGRES_PASSWORD first.')
text = path.read_text()
key = 'ARTEX_CODEX_GATEWAY_TOKEN'
values = dict(line.split('=', 1) for line in text.splitlines() if line and not line.startswith('#') and '=' in line)
if len(values.get(key, '').strip()) < 24:
    token = secrets.token_urlsafe(32)
    if key in values:
        text = '\n'.join(key + '=' + token if line.startswith(key + '=') else line for line in text.splitlines()) + '\n'
    else:
        text += '\n' + key + '=' + token + '\n'
    path.write_text(text)
    os.chmod(path, 0o600)
PY
docker compose -f docker-compose.yml -f docker-compose.codex.yml up -d --build
