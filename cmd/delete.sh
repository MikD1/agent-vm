# shellcheck shell=bash
# vm delete — stop and delete a VM.

cmd_delete() {
  preflight
  local force=0 name=""
  for arg in "$@"; do
    case "$arg" in
      -f|--force) force=1 ;;
      -*) die "unknown option: $arg" ;;
      *) name="$arg" ;;
    esac
  done

  name="$(resolve_target_name "$name")"
  validate_name "$name"

  if ! vm_exists "$name"; then
    info "VM '$name' does not exist."
    return 0
  fi

  if [[ "$force" -ne 1 ]]; then
    printf 'Delete VM "%s"? This is irreversible. [y/N] ' "$name"
    local reply
    read -r reply
    [[ "$reply" =~ ^[Yy]$ ]] || { info "Aborted."; return 0; }
  fi

  info "Stopping VM: $name"
  limactl stop "$name" 2>/dev/null || true
  info "Deleting VM: $name"
  limactl delete -f "$name"
  info "Deleted: $name"
}
