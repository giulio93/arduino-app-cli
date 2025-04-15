import json
import os
from pathlib import Path

metadata_file = "/app/.cache/metadata.json"
output_dir = "/app/.cache/compose"

if not os.path.exists(output_dir):
    os.makedirs(output_dir)

if os.path.exists(metadata_file):
    with open(metadata_file, "r") as f:
        data = f.read()
else:
    print(f"Error: {metadata_file} not found.")
    exit(1)

try:
    parsed_data = json.loads(data)
except json.JSONDecodeError:
    print(f"Error: Invalid JSON in {metadata_file}")
    exit(1)

for item in parsed_data:
    if "compose_file" not in item:
        continue

    compose_file_path = item["compose_file"]
    output_folder = os.path.join(output_dir, item["name"])
    Path(output_folder).mkdir(parents=True, exist_ok=True)
    if not os.path.exists(compose_file_path):
        continue

    with open(compose_file_path, "r") as f:
        compose_content = f.read()

    output_file_name = os.path.join(output_folder, "module_compose.yaml")
    with open(output_file_name, "w") as f:
        f.write(compose_content)
