#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

if ! command -v dotnet >/dev/null 2>&1; then
  curl -fsSL https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh
  chmod +x /tmp/dotnet-install.sh

  mkdir -p /opt/dotnet
  /tmp/dotnet-install.sh --channel LTS --install-dir /opt/dotnet

  ln -sf /opt/dotnet/dotnet /usr/local/bin/dotnet

  cat > /etc/profile.d/dotnet.sh <<'DOTNETEOF'
export DOTNET_ROOT=/opt/dotnet
export PATH="$DOTNET_ROOT:$PATH"
DOTNETEOF
fi
