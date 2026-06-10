# shellcheck shell=bash
# vm start — start a stopped VM.

cmd_start() {
  preflight
  local name
  name="$(resolve_target_name "${1:-}")"
  validate_name "$name"
  vm_exists "$name" || die "VM '$name' does not exist"
  limactl start "$name"
}
