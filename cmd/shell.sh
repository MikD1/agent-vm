# shellcheck shell=bash
# vm shell — open a shell in the VM.

cmd_shell() {
  preflight
  local name
  name="$(resolve_target_name "${1:-}")"
  validate_name "$name"
  vm_exists "$name" || die "VM '$name' does not exist"
  exec limactl shell "$name" "${@:2}"
}
