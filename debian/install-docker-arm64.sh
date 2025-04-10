#!/bin/sh
set -e

BASE_URL="https://download.docker.com/linux/debian/dists/bookworm/pool/stable/arm64"

cd /tmp

apt-get update

wget "$BASE_URL/containerd.io_1.7.27-1_arm64.deb"
apt-get install ./containerd.io_1.7.27-1_arm64.deb

wget "$BASE_URL/docker-ce_28.0.4-1~debian.12~bookworm_arm64.deb"
apt-get install ./docker-ce_28.0.4-1~debian.12~bookworm_arm64.deb

wget "$BASE_URL/docker-compose-plugin_2.34.0-1~debian.12~bookworm_arm64.deb"
apt-get install ./docker-compose-plugin_2.34.0-1~debian.12~bookworm_arm64.deb

echo "Docker installed successfully."

