#!/bin/bash
set -ex

BASE_DIR="/app"
PYTHON_SCRIPT="$BASE_DIR/main.py"
REQUIREMENTS_FILE="$BASE_DIR/requirements.txt"

start_virtualenv() {
  #TODO: we should support multiple python versions
  if [ ! -d "$BASE_DIR/.venv" ]; then
    echo "Creating virtualenv..."
    python3 -m venv "$BASE_DIR/.venv"
  fi
  source "$BASE_DIR/.venv/bin/activate"
}

install_requirements() {
  mkdir -p /tmp
  pip freeze | sort > /tmp/a.txt
  sort "$REQUIREMENTS_FILE" > /tmp/b.txt

  if cmp -s /tmp/a.txt /tmp/b.txt; then
    echo "Requirements unchanged, skipping pip install."
  else
    echo "Requirements changed, installing..."
    pip install -r "$REQUIREMENTS_FILE"
  fi
  rm /tmp/a.txt /tmp/b.txt
}

start_python() {
  echo "Starting Python script $PYTHON_SCRIPT..."
  python "$PYTHON_SCRIPT" &
}

kill_python() {
  pkill -f "$PYTHON_SCRIPT"
}

# Initial start
start_virtualenv
install_requirements
start_python

# Watch for file changes
inotifywait -m -e modify -e create -e delete -e move -e attrib $BASE_DIR | while read event; do
  echo "Change detected: $event"
  kill_python
  install_requirements
  start_python
done

kill_python
