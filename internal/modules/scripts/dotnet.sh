#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

if ! command -v dotnet >/dev/null 2>&1; then
  curl -fsSL https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh
  chmod +x /tmp/dotnet-install.sh

  mkdir -p /opt/dotnet
  # TMPDIR redirects dotnet-install.sh's extraction to /var/tmp (on the main
  # disk) instead of the default /tmp tmpfs which is too small for the SDK.
  TMPDIR=/var/tmp /tmp/dotnet-install.sh --channel LTS --install-dir /opt/dotnet

  ln -sf /opt/dotnet/dotnet /usr/local/bin/dotnet

  # Login shells (SSH, VS Code) source profile.d; this handles PATH.
  cat > /etc/profile.d/dotnet.sh <<'DOTNETEOF'
export DOTNET_ROOT=/opt/dotnet
export PATH="$DOTNET_ROOT:$PATH"
DOTNETEOF

  # `vm shell` runs a non-login shell that does NOT source profile.d, so also
  # record DOTNET_ROOT in /etc/environment (read by PAM for SSH sessions),
  # written idempotently. PATH isn't needed here — the symlink above puts the
  # dotnet binary on the default PATH, and /etc/environment can't expand $PATH.
  touch /etc/environment
  sed -i '/^DOTNET_ROOT=/d' /etc/environment
  echo 'DOTNET_ROOT=/opt/dotnet' >> /etc/environment
fi
