#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

if ! command -v go >/dev/null 2>&1; then
  # Go ships no apt/LTS channel and Ubuntu's golang-go package lags well behind,
  # so install the current stable release straight from go.dev, extracted to
  # /usr/local the way go.dev's own install instructions assume.
  #
  # Capture the version with a plain assignment (not `curl | head`): piping into
  # head closes the pipe early, which under `set -o pipefail` turns the curl
  # SIGPIPE into a failure (the same trap documented in lib/common.sh). The
  # endpoint returns the version on line 1 and a timestamp on line 2.
  GO_VERSION="$(curl -fsSL 'https://go.dev/VERSION?m=text')"
  GO_VERSION="${GO_VERSION%%$'\n'*}"
  [ -n "$GO_VERSION" ] || { echo "Error: could not determine latest Go version" >&2; exit 1; }

  # Go's tarball arch names (amd64/arm64) match dpkg's, so no translation needed.
  ARCH="$(dpkg --print-architecture)"
  TARBALL="${GO_VERSION}.linux-${ARCH}.tar.gz"

  # The tarball is unsigned, and go.dev serves no flat <file>.sha256 (that path
  # returns an HTML redirect with HTTP 200, not a digest, so `curl -f` can't
  # catch it). Pull the expected digest from go.dev's JSON release index instead,
  # matched to the exact filename, and sanity-check that it really is a SHA-256
  # before trusting it — so a future endpoint change fails loudly here rather
  # than feeding garbage to sha256sum. jq comes from the base module (runs first).
  EXPECTED="$(curl -fsSL 'https://go.dev/dl/?mode=json' \
    | jq -r --arg f "$TARBALL" '.[].files[] | select(.filename == $f) | .sha256')"
  [[ "$EXPECTED" =~ ^[a-f0-9]{64}$ ]] \
    || { echo "Error: no valid SHA-256 for $TARBALL from go.dev's release index" >&2; exit 1; }

  # Download to /var/tmp (main disk); /tmp is a small tmpfs the ~150MB archive
  # can overflow (the same gotcha the dotnet module hit).
  ARCHIVE="/var/tmp/${TARBALL}"
  curl -fsSL "https://go.dev/dl/${TARBALL}" -o "$ARCHIVE"

  # Verify the download against the digest from the index before extracting.
  echo "${EXPECTED}  ${ARCHIVE}" | sha256sum -c -

  # go.dev's instructions: remove any prior tree, then extract fresh.
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "$ARCHIVE"
  rm -f "$ARCHIVE"

  # Symlink the toolchain onto the default PATH so `go` works in every shell,
  # including the non-login shell `vm shell` opens (mirrors the dotnet module's
  # /usr/local/bin symlink).
  ln -sf /usr/local/go/bin/go /usr/local/bin/go
  ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

  # Login shells (SSH, VS Code) source profile.d. Add the default `go install`
  # bin dir ($HOME/go/bin) to PATH there so user-installed tools run by name;
  # include the toolchain dir too so GOROOT discovery is unambiguous. (The
  # symlinks above already cover non-login `vm shell`, which skips profile.d.)
  cat > /etc/profile.d/go.sh <<'GOEOF'
export PATH="/usr/local/go/bin:$PATH"
export PATH="$HOME/go/bin:$PATH"
GOEOF
fi
