#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

# Install Node from NodeSource. The old setup_lts.x convenience script is
# deprecated, so configure the apt repo directly. NodeSource's per-distro repos
# have no "lts" alias, so we pin a major version; bump NODE_MAJOR to track a
# newer LTS line. The key is stored ASCII-armored (signed-by=*.asc) so we don't
# need gnupg in the base image.
NODE_MAJOR=24
if ! command -v node >/dev/null 2>&1; then
  install -d -m 0755 /etc/apt/keyrings
  curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
    -o /etc/apt/keyrings/nodesource.asc
  echo "deb [signed-by=/etc/apt/keyrings/nodesource.asc] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
    > /etc/apt/sources.list.d/nodesource.list
  apt-get update
  apt-get install -y nodejs

  # Enable pnpm/yarn via corepack. Surface failures instead of swallowing them
  # with `|| true`; don't abort the module (Node is the hard requirement,
  # pnpm/yarn are best-effort).
  corepack enable || echo "Warning: 'corepack enable' failed; pnpm/yarn may be unavailable" >&2
  corepack install --global pnpm@latest || echo "Warning: failed to install pnpm via corepack" >&2
  corepack install --global yarn@stable || echo "Warning: failed to install yarn via corepack" >&2
fi
