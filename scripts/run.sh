#!/bin/sh

BASE_DIR="/app"
PYTHON_SCRIPT="$BASE_DIR/python/main.py"
REQUIREMENTS_FILE="$BASE_DIR/python/requirements.txt"

export UV_CACHE_DIR=/app/.cache/uv

uv venv "$BASE_DIR/.cache/.venv" --system-site-packages
. "$BASE_DIR/.cache/.venv/bin/activate"
uv pip install -r "$REQUIREMENTS_FILE"

if [ "$1" = "provision" ]; then
  appslab-list-modules > /app/.cache/metadata.json
  python /provision.py
else
  uv run --with-requirements "$REQUIREMENTS_FILE" "$PYTHON_SCRIPT"
fi
