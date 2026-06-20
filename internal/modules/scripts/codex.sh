#!/usr/bin/env bash
set -euo pipefail

# Install OpenAI Codex CLI via npm. The scoped package @openai/codex downloads
# the platform Rust binary on postinstall; the unscoped `codex` is an unrelated
# project. Requires Node >= 22, satisfied by the node module (installs Node 24).
command -v npm >/dev/null 2>&1 || { echo "Error: codex module requires npm (add node module first)"; exit 1; }
npm install -g @openai/codex

# Seed optional config from the host secrets dir. Both files are optional and
# copied verbatim into the VM user's ~/.codex. config.toml carries settings and
# MCP servers ([mcp_servers.<id>] tables); auth.json holds credentials, so it is
# chmod 600. This module runs standalone in the VM and cannot use lib/common.sh
# helpers, so it uses plain echo/exit like the claude module.
CODEX_DIR="${VM_SECRETS}/modules/codex"
CONFIG_SRC="${CODEX_DIR}/config.toml"
AUTH_SRC="${CODEX_DIR}/auth.json"
if [ -f "$CONFIG_SRC" ] || [ -f "$AUTH_SRC" ]; then
  sudo -u "${VM_USER}" bash -c "
    mkdir -p \"\$HOME/.codex\"
    if [ -f '$CONFIG_SRC' ]; then
      cp '$CONFIG_SRC' \"\$HOME/.codex/config.toml\"
    fi
    if [ -f '$AUTH_SRC' ]; then
      cp '$AUTH_SRC' \"\$HOME/.codex/auth.json\"
      chmod 600 \"\$HOME/.codex/auth.json\"
    fi
  "
fi
