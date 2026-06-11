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

validate_name() {
  [[ "$1" =~ ^[a-zA-Z0-9_-]+$ ]] || die "project name must contain only letters, digits, hyphens, or underscores: $1"
}

# Resolve the VM name when no argument was given: require .ai-dev-vm.yaml in cwd.
resolve_name_from_cwd() {
  [[ -f "$PWD/.ai-dev-vm.yaml" ]] || die "no .ai-dev-vm.yaml in current directory; pass a project name or cd into a project"
  local name
  name=$(basename "$PWD")
  validate_name "$name"
  printf '%s' "$name"
}

# Use $1 if non-empty, else resolve from cwd. Echoes the name.
resolve_target_name() {
  local name="${1:-}"
  if [[ -n "$name" ]]; then
    printf '%s' "$name"
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
  if [[ ! -f "$module_file" ]]; then
    warn "module '$mod' not found at $module_file, skipping"
    return 0
  fi
  info "Running module: $mod"
  # shellcheck disable=SC2016
  limactl shell "$vm_name" sudo bash -c '
    export VM_USER="$1" VM_PROJECT="$2" VM_SECRETS="$3"
    export DEBIAN_FRONTEND=noninteractive
    bash -euo pipefail -s
  ' -- "$vm_user" "$project_name" '/mnt/host/ai-dev-vm' < "$module_file"
}
