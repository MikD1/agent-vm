# shellcheck shell=bash
# shellcheck disable=SC2154  # REPO_DIR is exported by bin/vm
# Shared helpers for the vm CLI. Sourced by bin/vm.

info() { printf '%s\n' "$*"; }
warn() { printf 'Warning: %s\n' "$*" >&2; }
die()  { printf 'Error: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
vm — isolated Lima dev VMs, one per project

Usage:
  vm init [path]        Write a .ai-dev-vm.yaml template (default: current dir)
  vm create [path]      Create + start a VM from a project dir (default: current dir)
  vm list               List all Lima VMs
  vm shell [name]       Open a shell in the VM
  vm start [name]       Start a stopped VM
  vm stop [name]        Stop a running VM
  vm restart [name]     Restart a VM
  vm delete [name]      Stop and delete a VM (--force to skip confirmation)
  vm help               Show this help

When [name]/[path] is omitted, the current directory must contain a
.ai-dev-vm.yaml; the VM name is that directory's basename.
EOF
}

# Verify host tools needed for VM operations.
preflight() {
  command -v limactl >/dev/null 2>&1 || die "limactl not found. Install with: brew install lima"
  command -v yq >/dev/null 2>&1 || die "yq not found. Install with: brew install yq"
}

# Normalize a raw name (e.g. a directory basename) into a Lima-valid lowercase
# DNS label: lowercase, every char outside [a-z0-9-] becomes '-', then strip
# leading/trailing '-'. Fails (returns 1) if nothing valid remains.
normalize_name() {
  local n
  n="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9-' '-')"
  n="${n#"${n%%[!-]*}"}"   # strip leading hyphens
  n="${n%"${n##*[!-]}"}"   # strip trailing hyphens
  if [[ -z "$n" ]]; then
    printf 'Error: cannot derive a valid VM name from: %s\n' "$1" >&2
    return 1
  fi
  printf '%s' "$n"
}

# Assert a name is a valid Lima instance name (lowercase DNS label). Safety net:
# callers should pass names through normalize_name first.
validate_name() {
  if ! [[ "$1" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]]; then
    printf 'Error: VM name must be a lowercase DNS label (a-z, 0-9, hyphen; not starting or ending with '"'"'-'"'"'): %s\n' "$1" >&2
    return 1
  fi
}

# Assert a value is a positive integer (Lima's `cpus`). Prints a friendly error
# and returns 1 on failure (callers run it bare under `set -e`, like validate_name).
validate_cpus() {
  if ! [[ "$1" =~ ^[1-9][0-9]*$ ]]; then
    printf 'Error: invalid cpus %q in .ai-dev-vm.yaml (expected a positive integer)\n' "$1" >&2
    return 1
  fi
}

# Assert a value is a Lima size string (memory/disk), e.g. 4GiB, 512MiB, 8G.
# $1 = field name (for the message), $2 = value.
validate_size() {
  if ! [[ "$2" =~ ^[0-9]+(\.[0-9]+)?[KMGT](iB|B)?$ ]]; then
    printf 'Error: invalid %s %q in .ai-dev-vm.yaml (expected a size like 16GiB)\n' "$1" "$2" >&2
    return 1
  fi
}

# Resolve the VM name when no argument was given: require .ai-dev-vm.yaml in cwd.
resolve_name_from_cwd() {
  [[ -f "$PWD/.ai-dev-vm.yaml" ]] || die "no .ai-dev-vm.yaml in current directory; pass a project name or cd into a project"
  local name
  name="$(normalize_name "$(basename "$PWD")")"
  validate_name "$name"  # defense-in-depth: normalize_name output should always pass
  printf '%s' "$name"
}

# Use $1 if non-empty, else resolve from cwd. Echoes the name.
resolve_target_name() {
  local name="${1:-}"
  if [[ -n "$name" ]]; then
    normalize_name "$name"
  else
    resolve_name_from_cwd
  fi
}

vm_exists() {
  # Capture output first, then match. Piping limactl straight into `grep -q`
  # lets grep short-circuit on a match and close the pipe, killing limactl with
  # SIGPIPE; under `set -o pipefail` that turns the whole pipeline non-zero, so
  # an existing VM is falsely reported as missing.
  local names
  names="$(limactl list --format '{{.Name}}' 2>/dev/null)"
  grep -qxF -- "$1" <<<"$names"
}

# Echo the guest path where the project is mounted: the writable mount whose
# mountPoint ends in the VM name (create.sh mounts the project at
# "<guest_home>/<project_name>"). The read-only "/mnt/host/<name>" secrets mount
# and Lima's own writable mounts (e.g. /tmp/lima) are excluded. Echoes nothing
# if it can't be determined, so callers can fall back to Lima's default workdir.
vm_project_dir() {
  local name="$1" dir lima_yaml
  dir="$(limactl list --format '{{.Dir}}' "$name" 2>/dev/null)" || return 0
  lima_yaml="$dir/lima.yaml"
  [[ -f "$lima_yaml" ]] || return 0
  NAME="$name" yq -r '
    [ .mounts[]
      | select(.writable == true and (.mountPoint | test("/" + strenv(NAME) + "$")))
      | .mountPoint ] | .[0] // ""
  ' "$lima_yaml" 2>/dev/null
}

# Run a module script inside the VM as root via stdin.
run_module() {
  local vm_name="$1" vm_user="$2" project_name="$3" mod="$4"
  [[ "$mod" =~ ^[a-zA-Z0-9_-]+$ ]] || die "invalid module name: $mod"
  local module_file="$REPO_DIR/modules/${mod}.sh"
  [[ -f "$module_file" ]] || die "module '$mod' not found at $module_file"
  info "Running module: $mod"
  # --workdir / pins the guest cwd to a path that exists; without it `limactl
  # shell` tries to cd into the host's $PWD inside the guest and emits
  # "cd: <host path>: No such file or directory" noise (see cmd_shell). Modules
  # use absolute paths, so the cwd itself doesn't matter.
  # shellcheck disable=SC2016
  limactl shell --workdir / "$vm_name" sudo bash -c '
    export VM_USER="$1" VM_PROJECT="$2" VM_SECRETS="$3"
    export DEBIAN_FRONTEND=noninteractive
    bash -euo pipefail -s
  ' -- "$vm_user" "$project_name" '/mnt/host/ai-dev-vm' < "$module_file"
}
