#!/usr/bin/env bash
set -euo pipefail
# Phase 1 — system layer. Runs as root before any module.
# Installs host CA certificates into the system trust store and exports trust
# env globally, so every later tool/module inherits trust with no per-module code.
# Contract: VM_USER, VM_PROJECT, VM_WORKSPACE, VM_SECRETS (see architecture §5).

export DEBIAN_FRONTEND=noninteractive

CA_DIR="${VM_SECRETS}/ca-certificates"
CA_BUNDLE=/etc/ssl/certs/agent-vm-ca-bundle.pem

if [ -d "$CA_DIR" ]; then
  : > "$CA_BUNDLE.tmp"
  ca_found=0
  for cert in "$CA_DIR"/*.pem; do
    [ -f "$cert" ] || continue   # empty dir / no *.pem: glob stays literal, skip
    ca_found=1
    cp "$cert" "/usr/local/share/ca-certificates/$(basename "$cert" .pem).crt"
    cat "$cert" >> "$CA_BUNDLE.tmp"
  done
  if [ "$ca_found" = 1 ]; then
    update-ca-certificates
    mv "$CA_BUNDLE.tmp" "$CA_BUNDLE"

    # Login shells (SSH, VS Code) source /etc/profile.d.
    cat > /etc/profile.d/agent-vm-ca.sh <<EOF
export NODE_EXTRA_CA_CERTS="$CA_BUNDLE"
export SSL_CERT_FILE="$CA_BUNDLE"
export REQUESTS_CA_BUNDLE="$CA_BUNDLE"
export GIT_SSL_CAINFO="$CA_BUNDLE"
export CURL_CA_BUNDLE="$CA_BUNDLE"
EOF

    # Non-login shells (limactl shell) read /etc/environment via PAM. Write each
    # var idempotently.
    touch /etc/environment
    for kv in \
      "NODE_EXTRA_CA_CERTS=$CA_BUNDLE" \
      "SSL_CERT_FILE=$CA_BUNDLE" \
      "REQUESTS_CA_BUNDLE=$CA_BUNDLE" \
      "GIT_SSL_CAINFO=$CA_BUNDLE" \
      "CURL_CA_BUNDLE=$CA_BUNDLE"; do
      key="${kv%%=*}"
      sed -i "/^${key}=/d" /etc/environment
      echo "$kv" >> /etc/environment
    done
  else
    rm -f "$CA_BUNDLE.tmp"
  fi
fi
