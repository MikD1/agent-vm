#!/usr/bin/env bash
set -euo pipefail

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

# Ubuntu names fd as fdfind — create symlink
ln -sf "$(command -v fdfind)" /usr/local/bin/fd || true

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
    # `vm shell` runs a non-login shell that skips profile.d; also record it in
    # /etc/environment (read by PAM for SSH sessions), written idempotently.
    touch /etc/environment
    sed -i '/^NODE_EXTRA_CA_CERTS=/d' /etc/environment
    echo "NODE_EXTRA_CA_CERTS=$CA_BUNDLE" >> /etc/environment
  else
    rm -f "$CA_BUNDLE.tmp"
  fi
fi

# Copy gitconfig from host, stripping all credential sections (helpers + stored
# creds). Done with `git config` so it understands INI sections/subsections,
# unlike a line-based grep.
if [ -f "${VM_SECRETS}/.gitconfig" ]; then
  sudo -u "${VM_USER}" env VM_SECRETS="${VM_SECRETS}" bash -c '
    set -euo pipefail
    dest="$HOME/.gitconfig"
    cp "${VM_SECRETS}/.gitconfig" "$dest"
    # Top-level [credential] section.
    git config --file "$dest" --remove-section credential 2>/dev/null || true
    # Any [credential "<url>"] subsections: derive each subsection name from its
    # keys (credential.<url>.<key>) and remove the section.
    while IFS= read -r key; do
      sub="${key#credential.}"   # <url>.<key>
      sub="${sub%.*}"            # <url>
      [ -n "$sub" ] || continue
      git config --file "$dest" --remove-section "credential.$sub" 2>/dev/null || true
    done < <(git config --file "$dest" --name-only --get-regexp "^credential\." 2>/dev/null || true)
  '
fi
