#!/usr/bin/env bash
set -euo pipefail
# Phase 1 — system layer. Runs as root before any platform step or tool.
# Installs host CA certificates into the system trust store and exports trust
# env globally, so every later platform step (base, docker, mise) and every
# mise-installed tool inherits trust with no per-tool code.
# Contract: VM_USER, VM_HOME, VM_CONFIG (see architecture §5).
#
# Everything this script says goes to stderr: avm buffers the guest's stdout and
# streams only stderr, so a phase that is silent on stdout is silent on screen.
# A certificate that was not picked up has to be visible here — the alternative
# is what it used to be, a phase that quietly did nothing and a TLS failure three
# phases later, when the VM is already being rolled back.

export DEBIAN_FRONTEND=noninteractive

CA_DIR="${VM_CONFIG}/ca-certificates"
# The host-provided CAs alone. This is the bundle for consumers that ADD to
# their own root list rather than replace it — NODE_EXTRA_CA_CERTS is the one
# that matters, and handing it the merged store would be wrong.
CA_BUNDLE=/etc/ssl/certs/agent-vm-ca-bundle.pem
# The system store: the host CAs plus the public roots, rebuilt by
# update-ca-certificates. This is what the replace-the-root-list variables get.
SYSTEM_BUNDLE=/etc/ssl/certs/ca-certificates.crt
STAGE=/usr/local/share/ca-certificates

# avm_to_pem <file> — print the certificate(s) in <file> as PEM, one per block,
# each line newline-terminated; print nothing when the file holds no certificate.
#
# Two things this absorbs, both of which used to break a bundle silently. A
# corporate CA is handed out as often in DER (a .crt/.cer straight out of a
# Windows trust store) as in PEM, and update-ca-certificates only understands
# PEM. And a PEM exported from a browser or pasted through Windows tooling
# routinely carries CRLF line endings or no trailing newline at all — concatenate
# one of those and the next certificate's -----BEGIN----- lands on the same line
# as the previous -----END-----, which makes every certificate after the first
# unparseable to OpenSSL and invisible to rustls.
#
# The PEM branch is awk rather than `openssl x509`, deliberately: openssl reads
# only the first certificate out of a file, and a corporate CA file is often a
# chain (root plus one or more subordinates), all of which must be trusted.
avm_to_pem() {
  if grep -qa -- '-----BEGIN CERTIFICATE-----' "$1"; then
    awk '/-----BEGIN CERTIFICATE-----/,/-----END CERTIFICATE-----/' "$1" | tr -d '\r'
  else
    openssl x509 -inform der -in "$1" -outform pem 2>/dev/null || true
  fi
}

installed=0
rejected=0
rm -f "$CA_BUNDLE.tmp"

if [ -d "$CA_DIR" ]; then
  # Anything that is a certificate is taken, whatever it is called: .pem, .crt,
  # .cer, .der, no extension at all. The old glob matched *.pem only, so the
  # single most common thing a user has to hand — corp-root.crt — was skipped
  # without a word.
  install -d -m 0755 "$STAGE"
  rm -f "$STAGE"/avm-*.crt
  : > "$CA_BUNDLE.tmp"
  shopt -s nullglob
  for src in "$CA_DIR"/*; do
    [ -f "$src" ] || continue
    name="${src##*/}"
    case "$name" in
      .*|*.md) continue ;;   # a README next to the certificates is not an error
    esac
    pem="$(avm_to_pem "$src")"
    if [ -z "$pem" ]; then
      echo "system: WARNING ${name} is not a PEM or DER certificate; skipped" >&2
      rejected=$((rejected + 1))
      continue
    fi
    # update-ca-certificates only reads *.crt, and it identifies a certificate by
    # its filename — so the staged name has to stay unique per source file.
    # Dropping the extension would collide corp.pem with corp.crt and trust only
    # one of them; keeping it whole cannot.
    printf '%s\n' "$pem" > "$STAGE/avm-${name%.crt}.crt"
    printf '%s\n' "$pem" >> "$CA_BUNDLE.tmp"
    subject="$(printf '%s\n' "$pem" | openssl x509 -noout -subject 2>/dev/null || true)"
    echo "system: trusting ${name} ${subject:-(subject unavailable)}" >&2
    installed=$((installed + 1))
  done
  shopt -u nullglob
fi

if [ "$installed" -gt 0 ]; then
  if ! command -v update-ca-certificates >/dev/null 2>&1; then
    echo "Error: update-ca-certificates is missing; this base image cannot install CA certificates" >&2
    exit 1
  fi
  update-ca-certificates >&2
  mv "$CA_BUNDLE.tmp" "$CA_BUNDLE"

  # Trust must be ADDED to the public roots, never substituted for them. Pointing
  # SSL_CERT_FILE at the host CAs alone (which is what this did) leaves a VM that
  # can only talk to hosts the corporate proxy re-signs: the moment one host is on
  # the proxy's inspection-bypass list, its perfectly ordinary public certificate
  # chains to a root the VM no longer knows about, and the tool reports the same
  # UnknownIssuer it would for a missing corporate CA. The merged system store has
  # both, so both cases verify.
  TRUST_BUNDLE="$SYSTEM_BUNDLE"
  [ -s "$TRUST_BUNDLE" ] || TRUST_BUNDLE="$CA_BUNDLE"

  # Login shells (SSH, VS Code) source /etc/profile.d.
  cat > /etc/profile.d/agent-vm-ca.sh <<EOF
export NODE_EXTRA_CA_CERTS="$CA_BUNDLE"
export SSL_CERT_FILE="$TRUST_BUNDLE"
export SSL_CERT_DIR="/etc/ssl/certs"
export REQUESTS_CA_BUNDLE="$TRUST_BUNDLE"
export GIT_SSL_CAINFO="$TRUST_BUNDLE"
export CURL_CA_BUNDLE="$TRUST_BUNDLE"
EOF

  # Non-login shells (limactl shell) read /etc/environment via PAM. Write each
  # var idempotently.
  touch /etc/environment
  for kv in \
    "NODE_EXTRA_CA_CERTS=$CA_BUNDLE" \
    "SSL_CERT_FILE=$TRUST_BUNDLE" \
    "SSL_CERT_DIR=/etc/ssl/certs" \
    "REQUESTS_CA_BUNDLE=$TRUST_BUNDLE" \
    "GIT_SSL_CAINFO=$TRUST_BUNDLE" \
    "CURL_CA_BUNDLE=$TRUST_BUNDLE"; do
    key="${kv%%=*}"
    sed -i "/^${key}=/d" /etc/environment
    echo "$kv" >> /etc/environment
  done

  echo "system: ${installed} CA certificate(s) installed into the trust store" >&2
elif [ "$rejected" -gt 0 ]; then
  # Files were put there on purpose and not one of them was a certificate.
  # Failing here costs a rollback; letting it through costs the same rollback in
  # phase 2 or 3, with an error that names TLS instead of naming the files.
  rm -f "$CA_BUNDLE.tmp"
  echo "Error: ${CA_DIR} holds ${rejected} file(s), none of them a PEM or DER certificate" >&2
  exit 1
else
  rm -f "$CA_BUNDLE.tmp"
  echo "system: no custom CA certificates in <vm-dir>/ca-certificates/" >&2
fi

# TLS preflight. Every tool in phase 3 is downloaded from these two hosts, and a
# network that intercepts TLS fails there — deep inside mise's own retry loop,
# reported as "invalid peer certificate: UnknownIssuer" against a package name.
# The same failure is one line here, next to the certificate count that explains
# it. It is a warning, not an error: a VM whose network is simply not up yet, or
# which reaches its tools through a mirror, still provisions.
if command -v curl >/dev/null 2>&1; then
  for url in https://github.com https://api.github.com; do
    if ! curl -fsS --connect-timeout 10 --max-time 20 -o /dev/null "$url" 2>/dev/null; then
      echo "system: WARNING cannot verify TLS to ${url}" >&2
      echo "system:          if this network inspects TLS, put its root CA (PEM or DER)" >&2
      echo "system:          in <vm-dir>/ca-certificates/ and re-run avm recreate" >&2
    fi
  done
fi
