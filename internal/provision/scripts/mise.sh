#!/usr/bin/env bash
set -euo pipefail
# Phase 2 — mise, the single tool manager behind every module.
# Installs mise system-wide and puts its shim directory into every shell context.
# Contract: MISE_VERSION plus VM_USER, VM_HOME, VM_CONFIG.
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

  # Take the compressed tarball, not the bare binary: the same mise is ~95 MB
  # unpacked, ~22 MB as .tar.xz and ~37 MB as .tar.gz. GitHub serves release
  # downloads from a separate CDN (release-assets.githubusercontent.com), the
  # first host this phase sequence touches that is neither an Ubuntu mirror nor
  # download.docker.com; where that path is throttled, the 4x smaller asset is
  # the difference between a phase that finishes and one that looks hung. xz
  # ships with the Ubuntu base image — fall back to gzip if an image drops it.
  ext=tar.xz
  command -v xz >/dev/null 2>&1 || ext=tar.gz

  asset="mise-${MISE_VERSION}-linux-${arch}.${ext}"
  base="https://github.com/jdx/mise/releases/download/${MISE_VERSION}"

  # Download to /var/tmp (main disk), not /tmp, which is a small tmpfs — the same
  # gotcha the previous dotnet and go modules hit.
  #
  # A bare `curl -fsSL` here is silent and unbounded: on a path that blocks or
  # throttles the CDN it waits forever, and since lima.Provision buffers the
  # guest's stdout and streams only stderr, Phase 2 prints nothing at all — the
  # run is indistinguishable from a hang, with no error to act on. So: bound the
  # connect; treat a transfer crawling under 8 KB/s for 30s as stuck and start
  # over; resume from what is already on disk (-C -) so a retry costs only the
  # remainder; and report on stderr, the stream that reaches the terminal live.
  fetch() {
    curl -fL \
      --connect-timeout 15 \
      --speed-limit 8192 --speed-time 30 \
      --retry 5 --retry-delay 3 --retry-all-errors \
      --continue-at - \
      --no-progress-meter \
      -o "$1" "$2"
  }

  # curl's own progress meter redraws one line with \r, and avm's log filter is
  # line-buffered — the whole meter would arrive as a single blob when the phase
  # ends. Print a real line every ~20s instead, so a slow download is visibly
  # alive. The ticker sleeps in short steps so that killing it below leaves no
  # child holding the SSH channel open for longer than a moment.
  progress_ticker() {
    tick=0
    while sleep 2; do
      tick=$((tick + 1))
      if [ $((tick % 10)) -eq 0 ] && [ -f "$1" ]; then
        echo "mise: $(du -m "$1" | cut -f1) MB so far" >&2
      fi
    done
  }

  echo "mise: downloading ${asset}" >&2
  progress_ticker "/var/tmp/${asset}" &
  ticker=$!
  trap 'kill "$ticker" 2>/dev/null || true' EXIT
  fetch_status=0
  fetch "/var/tmp/${asset}" "${base}/${asset}" || fetch_status=$?
  kill "$ticker" 2>/dev/null || true
  trap - EXIT

  if [ "$fetch_status" -ne 0 ]; then
    echo "Error: could not download ${base}/${asset}" >&2
    echo "       The VM has no usable path to GitHub release assets. Measure it with:" >&2
    echo "         curl -o /dev/null --max-time 30 -w 'bytes/sec: %{speed_download}' -L ${base}/${asset}" >&2
    if [ -s "/var/tmp/${asset}" ]; then
      echo "       $(du -m "/var/tmp/${asset}" | cut -f1) MB is on disk at /var/tmp/${asset}; the next run resumes from there." >&2
    fi
    exit 1
  fi
  fetch /var/tmp/mise-SHASUMS256.txt "${base}/SHASUMS256.txt"

  # Verify against the published digest before trusting the binary. Match the
  # filename with awk on the exact field rather than grep, so neither the dots in
  # the version nor the sibling "-musl" asset can match by accident. mise's
  # SHASUMS256.txt lists filenames "./"-prefixed (e.g. "./mise-v2026.8.4-linux-x64"),
  # so strip that prefix before comparing and re-print without it — the file on
  # disk at /var/tmp/${asset} has no "./" prefix, and sha256sum -c expects the
  # printed filename to match what it opens. Run from /var/tmp so it resolves.
  #
  # A resumed download that went wrong lands here, so discard the archive on a
  # mismatch: keeping it would poison every later run with the same failure.
  if ! (
    cd /var/tmp
    awk -v f="$asset" '{ n = $2; sub(/^\.\//, "", n) } n == f { print $1 "  " n; found = 1 } END { exit !found }' \
      mise-SHASUMS256.txt | sha256sum -c -
  ); then
    rm -f "/var/tmp/${asset}"
    echo "Error: ${asset} failed checksum verification; the file was discarded" >&2
    exit 1
  fi

  # The archive holds mise/bin/mise plus docs; only the binary is installed.
  tar -xf "/var/tmp/${asset}" -C /var/tmp
  install -m 0755 /var/tmp/mise/bin/mise /usr/local/bin/mise
  rm -rf "/var/tmp/${asset}" /var/tmp/mise-SHASUMS256.txt /var/tmp/mise
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
