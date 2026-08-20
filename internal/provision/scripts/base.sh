#!/usr/bin/env bash
set -euo pipefail
# Phase 2 — base packages, a platform step. Always installed, first among the
# Phase 2 platform steps (base, docker, mise); never selected by a project.
# Contract: VM_USER, VM_HOME, VM_CONFIG.
# Certificates are NOT handled here — the Phase 1 system layer already configured
# trust globally before this runs.

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y \
  ca-certificates \
  curl \
  git \
  jq \
  ripgrep \
  fd-find \
  build-essential

# Ubuntu names fd as fdfind — create a stable `fd` on PATH.
ln -sf "$(command -v fdfind)" /usr/local/bin/fd || true

# Copy gitconfig from the VM directory, stripping all credential sections
# (helpers + stored creds). Uses `git config` so it understands INI
# sections/subsections.
if [ -f "${VM_CONFIG}/.gitconfig" ]; then
  sudo -u "${VM_USER}" env VM_CONFIG="${VM_CONFIG}" bash -c '
    set -euo pipefail
    dest="$HOME/.gitconfig"
    cp "${VM_CONFIG}/.gitconfig" "$dest"
    git config --file "$dest" --remove-section credential 2>/dev/null || true
    while IFS= read -r key; do
      sub="${key#credential.}"   # <url>.<key>
      sub="${sub%.*}"            # <url>
      [ -n "$sub" ] || continue
      git config --file "$dest" --remove-section "credential.$sub" 2>/dev/null || true
    done < <(git config --file "$dest" --name-only --get-regexp "^credential\." 2>/dev/null || true)
  '
fi
