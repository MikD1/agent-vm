#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sh
fi

systemctl enable docker || true
systemctl start docker || true

if [ -n "${VM_USER:-}" ]; then
  usermod -aG docker "${VM_USER}" || true
fi
