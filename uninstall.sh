#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
CONFIG_DIR="$HOME/.config/ai-dev-vm"
BIN_DIR="$HOME/.local/bin"
VM_LINK="$BIN_DIR/vm"

# This removes what install.sh put on the system. It never touches third-party
# tools (lima, yq, brew, ...) or the repo itself — only ai-dev-vm's own traces.

# 1. Remove the vm launcher symlink, but only if it still points into this repo.
if [ -L "$VM_LINK" ]; then
  target="$(readlink "$VM_LINK")"
  case "$target" in
    "$REPO_DIR"/*)
      rm -f "$VM_LINK"
      echo "Removed $VM_LINK"
      ;;
    *)
      echo "Left $VM_LINK alone (points to $target, not this repo)"
      ;;
  esac
elif [ -e "$VM_LINK" ]; then
  echo "Left $VM_LINK alone (not a symlink)"
fi

# 2. Leave the config directory in place — it may hold a customized .gitconfig
#    or other things worth keeping. Just point it out.
if [ -d "$CONFIG_DIR" ]; then
  echo "Left $CONFIG_DIR in place (may hold useful config; delete it yourself if not)"
fi

# 3. Lima VMs created by `vm create` carry no marker, so we cannot tell them
#    apart from any other Lima instance — list them and let you decide.
if command -v limactl >/dev/null 2>&1; then
  vms="$(limactl list --format '{{.Name}}' 2>/dev/null || true)"
  if [ -n "$vms" ]; then
    echo
    echo "Lima VMs still present (delete any project ones yourself before they orphan):"
    printf '%s\n' "$vms" | sed 's/^/  /'
    echo "  Remove one with: limactl delete -f <name>"
  fi
fi

# 4. Summary.
echo
echo "Done. ai-dev-vm's traces are removed."
echo "The repo at $REPO_DIR was left in place — delete it with 'rm -rf' if you no longer need it."
