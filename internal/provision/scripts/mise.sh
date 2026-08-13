#!/usr/bin/env bash
set -euo pipefail
# Phase 2 — mise, the single tool manager behind every module.
# Installs mise system-wide and puts its shim directory into every shell context.
# Contract: MISE_VERSION plus VM_USER, VM_PROJECT, VM_WORKSPACE, VM_SECRETS.
# Certificates are NOT handled here — the Phase 1 system layer already configured
# trust globally, and mise honors the system store (its default build links
# native-tls, i.e. OpenSSL, on Linux).

export DEBIAN_FRONTEND=noninteractive

MISE_DATA_DIR=/usr/local/share/mise
SHIMS=/usr/local/share/mise/shims

if ! command -v mise >/dev/null 2>&1; then
  # mise's release assets use x64/arm64; dpkg says amd64/arm64.
  case "$(dpkg --print-architecture)" in
    amd64) arch=x64 ;;
    arm64) arch=arm64 ;;
    *) echo "Error: unsupported architecture $(dpkg --print-architecture)" >&2; exit 1 ;;
  esac

  asset="mise-${MISE_VERSION}-linux-${arch}"
  base="https://github.com/jdx/mise/releases/download/${MISE_VERSION}"

  # Download to /var/tmp (main disk), not /tmp, which is a small tmpfs — the same
  # gotcha the previous dotnet and go modules hit.
  curl -fsSL "${base}/${asset}" -o "/var/tmp/${asset}"
  curl -fsSL "${base}/SHASUMS256.txt" -o /var/tmp/mise-SHASUMS256.txt

  # Verify against the published digest before trusting the binary. Match the
  # filename with awk on the exact field rather than grep, so neither the dots in
  # the version nor the sibling "-musl" asset can match by accident. mise's
  # SHASUMS256.txt lists filenames "./"-prefixed (e.g. "./mise-v2026.8.4-linux-x64"),
  # so strip that prefix before comparing and re-print without it — the file on
  # disk at /var/tmp/${asset} has no "./" prefix, and sha256sum -c expects the
  # printed filename to match what it opens. Run from /var/tmp so it resolves.
  (
    cd /var/tmp
    awk -v f="$asset" '{ n = $2; sub(/^\.\//, "", n) } n == f { print $1 "  " n; found = 1 } END { exit !found }' \
      mise-SHASUMS256.txt | sha256sum -c -
  )

  install -m 0755 "/var/tmp/${asset}" /usr/local/bin/mise
  rm -f "/var/tmp/${asset}" /var/tmp/mise-SHASUMS256.txt
fi

# MISE_DATA_DIR must be exported globally, not only while provisioning: a shim is
# a symlink to mise, and mise re-resolves the install location on every call. With
# the variable missing it would look under the calling user's home and find nothing.

# Login shells (SSH, VS Code) source profile.d. Guard each prepend so repeated
# sourcing in nested shells cannot duplicate entries.
cat > /etc/profile.d/mise.sh <<'PROFILE_EOF'
export MISE_DATA_DIR=/usr/local/share/mise
for _avm_dir in "$MISE_DATA_DIR/shims" "$HOME/.local/bin" "$HOME/go/bin"; do
  case ":$PATH:" in
    *":$_avm_dir:"*) ;;
    *) PATH="$_avm_dir:$PATH" ;;
  esac
done
unset _avm_dir
export PATH
PROFILE_EOF

# Non-login shells (the shell `avm shell` opens) skip profile.d and read
# /etc/environment via PAM. PAM cannot expand variables, so PATH is written out in
# full. Rewrite only the keys we own; Lima maintains its own #LIMA-START/#LIMA-END
# block in this file and the order of key=value lines does not matter.
touch /etc/environment
sed -i '/^PATH=/d; /^MISE_DATA_DIR=/d' /etc/environment
{
  echo "PATH=\"${SHIMS}:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin\""
  echo "MISE_DATA_DIR=${MISE_DATA_DIR}"
} >> /etc/environment

# sudo ignores PATH and /etc/environment and uses secure_path from sudoers, so
# without this `sudo node` would not find a shim. Validate before installing: an
# invalid sudoers file breaks sudo entirely.
cat > /var/tmp/agent-vm-mise.sudoers <<SUDOERS_EOF
Defaults secure_path="${SHIMS}:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/snap/bin"
SUDOERS_EOF
visudo -cf /var/tmp/agent-vm-mise.sudoers
install -m 0440 -o root -g root /var/tmp/agent-vm-mise.sudoers /etc/sudoers.d/agent-vm-mise
rm -f /var/tmp/agent-vm-mise.sudoers
