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
  limactl list --format '{{.Name}}' 2>/dev/null | grep -qx "$1"
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
