#!/usr/bin/env bash
set -euo pipefail

# Requires node module to have run first
command -v npm >/dev/null 2>&1 || { echo "Error: claude module requires the node module"; exit 1; }

# Install Claude Code CLI
npm install -g @anthropic-ai/claude-code

# Create with-ai-env wrapper
sudo -u "${VM_USER}" bash -c '
  mkdir -p "$HOME/.local/bin"

  cat > "$HOME/.local/bin/with-ai-env" <<'"'"'WRAPPEREOF'"'"'
#!/usr/bin/env bash
set -euo pipefail

if [ -f /mnt/host/ai-dev/secrets.env ]; then
  set -a
  source /mnt/host/ai-dev/secrets.env
  set +a
fi

exec "$@"
WRAPPEREOF

  chmod +x "$HOME/.local/bin/with-ai-env"
'

# Add claude-env function and PATH to shell rc files
sudo -u "${VM_USER}" bash -c '
  BLOCK_MARKER="# ai-dev-vm:claude"

  add_rc_block() {
    local rc_file="$1"
    [ -f "$rc_file" ] || return 0
    grep -qF "$BLOCK_MARKER" "$rc_file" && return 0
    cat >> "$rc_file" <<'"'"'RCEOF'"'"'

# ai-dev-vm:claude
export PATH="$HOME/.local/bin:$PATH"

claude-env() (
  if [ -f /mnt/host/ai-dev/secrets.env ]; then
    set -a
    source /mnt/host/ai-dev/secrets.env
    set +a
  fi
  command claude "$@"
)
RCEOF
  }

  add_rc_block "$HOME/.bashrc"
  add_rc_block "$HOME/.zshrc"
'
