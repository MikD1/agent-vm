#!/usr/bin/env bash
set -euo pipefail

# Install OpenAI Codex CLI via npm. The scoped package @openai/codex downloads
# the platform Rust binary on postinstall; the unscoped `codex` is an unrelated
# project. Requires Node >= 22, satisfied by the node module (installs Node 24).
command -v npm >/dev/null 2>&1 || { echo "Error: codex module requires npm (add node module first)"; exit 1; }
npm install -g @openai/codex

# Write a minimal reference config: full-auto (never prompt), no sandbox.
# approval_policy="never" skips all approval prompts; sandbox_mode="danger-full-access"
# removes filesystem/network restrictions. Users can override by editing ~/.codex/config.toml
# inside the VM after provisioning.
sudo -u "${VM_USER}" -H bash -c "
  mkdir -p \"\$HOME/.codex\"
  cat > \"\$HOME/.codex/config.toml\" <<'EOF'
# Full-auto YOLO mode: no approval prompts, no sandbox.
approval_policy = \"never\"
sandbox_mode = \"danger-full-access\"
EOF
"

# Copy auth credentials if provided. auth.json is the file-based credential store
# written by `codex login`; without it the user must run `codex login` manually
# or set OPENAI_API_KEY. chmod 600 because it holds credentials.
AUTH_SRC="${VM_SECRETS}/modules/codex/auth.json"
if [ -f "$AUTH_SRC" ]; then
  sudo -u "${VM_USER}" -H bash -c "
    cp '$AUTH_SRC' \"\$HOME/.codex/auth.json\"
    chmod 600 \"\$HOME/.codex/auth.json\"
  "
fi
