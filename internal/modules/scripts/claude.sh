#!/usr/bin/env bash
set -euo pipefail

# Install Claude Code CLI via npm
command -v npm >/dev/null 2>&1 || { echo "Error: claude module requires npm (add node module first)"; exit 1; }
npm install -g @anthropic-ai/claude-code

# Copy settings.json if provided
CLAUDE_CONFIG="${VM_SECRETS}/modules/claude/settings.json"
if [ -f "$CLAUDE_CONFIG" ]; then
  sudo -u "${VM_USER}" bash -c "
    mkdir -p \"\$HOME/.claude\"
    cp '$CLAUDE_CONFIG' \"\$HOME/.claude/settings.json\"
  "
fi

# Install plugins if list provided. One token per line; "name" uses the default
# marketplace (claude-plugins-official), "name@marketplace" targets a specific
# one. `claude plugin install` defaults to --scope user. Run as the VM user with
# their HOME (-H) so plugins land in that user's ~/.claude. NOTE: this module
# runs standalone in the VM and cannot use lib/common.sh helpers, so failures
# are reported with a plain echo to stderr (not `warn`), and we do NOT swallow
# the exit code unconditionally.
PLUGINS_FILE="${VM_SECRETS}/modules/claude/plugins"
if [ -f "$PLUGINS_FILE" ]; then
  sudo -u "${VM_USER}" -H claude plugin marketplace add anthropics/claude-plugins-official 2>/dev/null || true
  sudo -u "${VM_USER}" -H claude plugin marketplace update claude-plugins-official || true
  while IFS= read -r plugin || [ -n "$plugin" ]; do
    [[ -z "$plugin" || "$plugin" =~ ^# ]] && continue
    echo "Installing Claude plugin: $plugin"
    if ! sudo -u "${VM_USER}" -H claude plugin install "$plugin"; then
      echo "Warning: Claude plugin failed to install: $plugin" >&2
    fi
  done < "$PLUGINS_FILE"
fi
