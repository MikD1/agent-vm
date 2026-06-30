#!/bin/sh
set -eu

REPO="MikD1/agent-vm"
GITHUB="https://github.com/${REPO}"
API="https://api.github.com/repos/${REPO}"

info() {
  printf '%s\n' "$*"
}

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

require_macos() {
  os_name=$(uname -s)
  if [ "$os_name" != "Darwin" ]; then
    die "install.sh supports macOS only"
  fi
}

detect_arch() {
  machine=$(uname -m)
  case "$machine" in
    arm64 | aarch64)
      printf 'arm64\n'
      ;;
    x86_64 | amd64)
      printf 'amd64\n'
      ;;
    *)
      die "unsupported macOS architecture: ${machine}"
      ;;
  esac
}

ensure_lima() {
  if command -v limactl >/dev/null 2>&1; then
    return
  fi

  if command -v brew >/dev/null 2>&1; then
    info "Lima not found; installing Lima with Homebrew..."
    brew install lima
    return
  fi

  die "Lima is required. Install Homebrew from https://brew.sh and rerun this script, or install Lima manually from https://lima-vm.io/."
}

resolve_version() {
  if [ -n "${AVM_VERSION:-}" ]; then
    printf '%s\n' "$AVM_VERSION"
    return
  fi

  latest_json=$(curl -fsSL "${API}/releases/latest") || die "failed to resolve latest avm release"
  version=$(printf '%s\n' "$latest_json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
  if [ -z "$version" ]; then
    die "failed to parse latest avm release"
  fi
  printf '%s\n' "$version"
}

install_dir() {
  if [ -n "${AVM_INSTALL_DIR:-}" ]; then
    printf '%s\n' "$AVM_INSTALL_DIR"
    return
  fi
  if [ -z "${HOME:-}" ]; then
    die "HOME is not set; set AVM_INSTALL_DIR to choose an install directory"
  fi
  printf '%s/.local/bin\n' "$HOME"
}

download_and_install() {
  arch=$1
  version=$2
  target_dir=$3

  version_name=${version#v}
  archive="avm_${version_name}_darwin_${arch}.tar.gz"
  tmpdir=$(mktemp -d)
  trap 'rm -rf "$tmpdir"' 0 HUP INT TERM

  archive_url="${GITHUB}/releases/download/${version}/${archive}"
  checksums_url="${GITHUB}/releases/download/${version}/checksums.txt"

  info "Downloading ${archive}..."
  curl -fsSL "$archive_url" -o "${tmpdir}/${archive}" || die "failed to download ${archive}"
  curl -fsSL "$checksums_url" -o "${tmpdir}/checksums.txt" || die "failed to download checksums.txt"

  checksum_line=$(awk -v file="$archive" '$2 == file { print; found = 1; exit } END { if (!found) exit 1 }' "${tmpdir}/checksums.txt" || true)
  if [ -z "$checksum_line" ]; then
    die "checksums.txt does not contain ${archive}"
  fi

  printf '%s\n' "$checksum_line" >"${tmpdir}/checksum"
  (
    cd "$tmpdir"
    shasum -a 256 -c checksum >/dev/null
  ) || die "checksum verification failed for ${archive}"

  mkdir -p "${tmpdir}/extract"
  tar -xzf "${tmpdir}/${archive}" -C "${tmpdir}/extract"
  avm_binary=$(find "${tmpdir}/extract" -type f -name avm -print | head -n 1)
  if [ -z "$avm_binary" ]; then
    die "${archive} did not contain an avm binary"
  fi

  mkdir -p "$target_dir"
  install -m 0755 "$avm_binary" "${target_dir}/avm"
}

path_hint() {
  target_dir=$1
  case ":${PATH:-}:" in
    *":${target_dir}:"*)
      ;;
    *)
      info "Add ${target_dir} to PATH to run avm from any shell."
      ;;
  esac
}

main() {
  require_macos
  ensure_lima

  arch=$(detect_arch)
  version=$(resolve_version)
  target_dir=$(install_dir)

  download_and_install "$arch" "$version" "$target_dir"
  info "Installed avm to ${target_dir}/avm"
  path_hint "$target_dir"
}

main "$@"
