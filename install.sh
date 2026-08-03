#!/bin/sh
set -eu

REPO="MikD1/agent-vm"
GITHUB="https://github.com/${REPO}"
API="https://api.github.com/repos/${REPO}"
RAW="https://raw.githubusercontent.com/${REPO}/main"

info() {
  printf '%s\n' "$*"
}

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

# http_status <url> — the HTTP status of a HEAD request, or 000 when there was no
# response at all (DNS, TLS, a proxy refusing to connect). Used only on a failure
# path: every other request runs with curl -f, which reports nothing but its own
# exit code, so the reason a download failed is otherwise invisible.
http_status() {
  curl -sSI -o /dev/null -w '%{http_code}\n' "$1" 2>/dev/null || printf '000\n'
}

host_os() {
  case "$(uname -s)" in
    Darwin)
      printf "darwin\n"
      ;;
    Linux)
      printf "linux\n"
      ;;
    *)
      die "install.sh supports macOS and Linux only"
      ;;
  esac
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
      die "unsupported architecture: ${machine}"
      ;;
  esac
}

ensure_lima() {
  host=$1
  if command -v limactl >/dev/null 2>&1; then
    return
  fi

  if [ "$host" = "darwin" ] && command -v brew >/dev/null 2>&1; then
    info "Lima not found; installing Lima with Homebrew..."
    brew install lima
    return
  fi

  if [ "$host" = "linux" ]; then
    die "Lima is required. Install Lima and QEMU with KVM access, then rerun this script. See https://lima-vm.io/."
  fi

  die "Lima is required. Install Homebrew from https://brew.sh and rerun this script, or install Lima manually from https://lima-vm.io/."
}

# resolve_version prints the release tag to install.
#
# github.com's own /releases/latest redirect is asked first, not the API.
# api.github.com is a different host, and a corporate proxy that passes
# github.com routinely blocks that one outright — its denial arrives as a 403 on
# a repository whose releases are public and downloadable one host over. GitHub's
# unauthenticated rate limit (60 requests/hour per IP) produces the same 403 from
# a shared corporate egress address. Neither has anything to do with the release,
# and neither needs the API: /releases/latest answers 302 to
# /releases/tag/<version>, which is the whole lookup. The API stays as the
# fallback for the reverse case, a network that passes the API but not the
# release pages.
resolve_version() {
  if [ -n "${AVM_VERSION:-}" ]; then
    printf '%s\n' "$AVM_VERSION"
    return
  fi

  # -I so the redirect costs a HEAD rather than the release page's HTML, and
  # url_effective for where the last hop landed.
  effective=$(curl -fsSLI -o /dev/null -w '%{url_effective}\n' "${GITHUB}/releases/latest" 2>/dev/null || true)
  case "$effective" in
    */releases/tag/?*)
      version=${effective##*/releases/tag/}
      printf '%s\n' "${version%/}"
      return
      ;;
  esac

  latest_json=$(curl -fsSL "${API}/releases/latest" 2>/dev/null || true)
  version=$(printf '%s\n' "$latest_json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
  if [ -n "$version" ]; then
    printf '%s\n' "$version"
    return
  fi

  # Both ways are gone. Say what each host answered — the status is the whole
  # diagnosis, and curl -f swallowed it — and give the way past release discovery.
  printf 'Error: failed to resolve the latest avm release\n' >&2
  printf '       %s/releases/latest -> HTTP %s\n' "$GITHUB" "$(http_status "${GITHUB}/releases/latest")" >&2
  printf '       %s/releases/latest -> HTTP %s\n' "$API" "$(http_status "${API}/releases/latest")" >&2
  printf '       A 403 from api.github.com alone is a proxy blocking that host, or\n' >&2
  printf '       GitHub rate-limiting an unauthenticated IP (60 requests/hour).\n' >&2
  printf '       Pin the version to skip release discovery entirely:\n' >&2
  printf '         curl -fsSL %s/install.sh | AVM_VERSION=<tag> sh\n' "$RAW" >&2
  printf '       Tags: %s/releases\n' "$GITHUB" >&2
  exit 1
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
  host=$1
  arch=$2
  version=$3
  target_dir=$4

  version_name=${version#v}
  archive="avm_${version_name}_${host}_${arch}.tar.gz"
  tmpdir=$(mktemp -d)
  trap 'rm -rf "$tmpdir"' 0 HUP INT TERM

  archive_url="${GITHUB}/releases/download/${version}/${archive}"
  checksums_url="${GITHUB}/releases/download/${version}/checksums.txt"

  info "Downloading ${archive}..."
  curl -fsSL "$archive_url" -o "${tmpdir}/${archive}" \
    || die "failed to download ${archive} (HTTP $(http_status "$archive_url"))"
  curl -fsSL "$checksums_url" -o "${tmpdir}/checksums.txt" \
    || die "failed to download checksums.txt (HTTP $(http_status "$checksums_url"))"

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
  host=$(host_os)
  ensure_lima "$host"

  arch=$(detect_arch)
  version=$(resolve_version)
  target_dir=$(install_dir)

  download_and_install "$host" "$arch" "$version" "$target_dir"
  info "Installed avm to ${target_dir}/avm"
  path_hint "$target_dir"
}

main "$@"
