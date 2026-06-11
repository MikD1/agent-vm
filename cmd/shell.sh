# shellcheck shell=bash
# vm shell — open a shell in the VM.

cmd_shell() {
  preflight
  local name
  name="$(resolve_target_name "${1:-}")"
  validate_name "$name"
  vm_exists "$name" || die "VM '$name' does not exist"

  # Open the shell in the project directory inside the VM when we can locate its
  # mount; otherwise fall back to Lima's default (the user's home), which also
  # avoids Lima's "cd: <host path>: No such file or directory" noise.
  local workdir
  workdir="$(vm_project_dir "$name")"
  if [[ -n "$workdir" ]]; then
    exec limactl shell --workdir "$workdir" "$name" "${@:2}"
  fi
  exec limactl shell "$name" "${@:2}"
}
