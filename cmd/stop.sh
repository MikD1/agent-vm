# shellcheck shell=bash
# vm stop — stop a running VM.

cmd_stop() {
  preflight
  local name
  name="$(resolve_target_name "${1:-}")"
  validate_name "$name"
  vm_exists "$name" || die "VM '$name' does not exist"
  limactl stop "$name"
}
