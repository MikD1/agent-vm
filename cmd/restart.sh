# shellcheck shell=bash
# vm restart — restart a VM.

cmd_restart() {
  preflight
  local name
  name="$(resolve_target_name "${1:-}")"
  validate_name "$name"
  vm_exists "$name" || die "VM '$name' does not exist"
  limactl restart "$name"
}
