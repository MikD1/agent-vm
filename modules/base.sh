#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y \
  ca-certificates \
  curl \
  wget \
  git \
  git-lfs \
  jq \
  ripgrep \
  fd-find \
  build-essential \
  pkg-config \
  gnupg \
  lsb-release \
  apt-transport-https \
  software-properties-common

# Ubuntu names fd as fdfind — create symlink
ln -sf "$(command -v fdfind)" /usr/local/bin/fd || true

git lfs install --system || true

# Install custom CA certificates if provided
CA_DIR="${VM_SECRETS}/ca-certificates"
if [ -d "$CA_DIR" ]; then
  CA_BUNDLE=/etc/ssl/certs/custom-ca-bundle.pem
  : > "$CA_BUNDLE.tmp"
  ca_found=0
  for cert in "$CA_DIR"/*.pem; do
    [ -f "$cert" ] || continue   # empty dir / no *.pem: glob stays literal, skip it
    ca_found=1
    cp "$cert" "/usr/local/share/ca-certificates/$(basename "$cert" .pem).crt"
    cat "$cert" >> "$CA_BUNDLE.tmp"
  done
  if [ "$ca_found" = 1 ]; then
    update-ca-certificates
    mv "$CA_BUNDLE.tmp" "$CA_BUNDLE"
    # Node.js/npm uses its own CA bundle; point it to the system bundle
    echo "export NODE_EXTRA_CA_CERTS=\"$CA_BUNDLE\"" > /etc/profile.d/custom-ca.sh
    export NODE_EXTRA_CA_CERTS="$CA_BUNDLE"
  else
    rm -f "$CA_BUNDLE.tmp"
  fi
fi

# Copy gitconfig from host, stripping credential helpers
if [ -f "${VM_SECRETS}/.gitconfig" ]; then
  sudo -u "${VM_USER}" env VM_SECRETS="${VM_SECRETS}" bash -c '
    grep -v -i "credential\." "${VM_SECRETS}/.gitconfig" > "$HOME/.gitconfig" || true
  '
fi
