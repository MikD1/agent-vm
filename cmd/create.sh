# shellcheck shell=bash
# shellcheck disable=SC2154  # REPO_DIR is exported by bin/vm
# vm create — create and start a Lima VM for a project.

# Roll back a half-built VM if cmd_create didn't finish, and remove the temp
# config. Installed as the EXIT trap inside cmd_create.
cleanup_create() {
  local rc=$?
  rm -f "$_CREATE_TMPFILE"
  if [[ "$_CREATE_DONE" -ne 1 && -n "$_CREATE_VM_NAME" ]]; then
    warn "vm create failed (see output above). Rolling back: deleting VM '$_CREATE_VM_NAME'."
    limactl delete -f "$_CREATE_VM_NAME" >/dev/null 2>&1 \
      || warn "rollback failed; remove it manually: vm delete $_CREATE_VM_NAME"
  fi
  exit "$rc"
}

cmd_create() {
  preflight

  local host_project_path="${1:-$PWD}"
  [[ -d "$host_project_path" ]] || die "project directory not found: $host_project_path"
  host_project_path="$(cd "$host_project_path" && pwd)"

  local project_name vm_name
  project_name="$(normalize_name "$(basename "$host_project_path")")"
  validate_name "$project_name"
  vm_name="$project_name"

  local config_file="$host_project_path/.ai-dev-vm.yaml"
  [[ -f "$config_file" ]] || die ".ai-dev-vm.yaml not found in $host_project_path (run: vm init)"

  if vm_exists "$vm_name"; then
    die "VM '$vm_name' already exists. The VM name is the project directory's basename, so two directories with the same basename (e.g. ~/work/api and ~/personal/api) map to the same VM. Delete it first: vm delete $project_name"
  fi

  # Read the module list once, then validate it BEFORE creating the VM so a typo
  # fails fast and never leaves a half-built VM (P9). Note whether docker is
  # requested — that's the only reason we restart at the end (P5).
  local modules_list needs_restart=0 mod
  modules_list="$(yq -r '.modules[]' "$config_file" 2>/dev/null || true)"
  while IFS= read -r mod; do
    [[ -z "$mod" || "$mod" == "null" ]] && continue
    [[ "$mod" =~ ^[a-zA-Z0-9_-]+$ ]] || die "invalid module name: $mod"
    [[ -f "$REPO_DIR/modules/${mod}.sh" ]] || die "unknown module '$mod' (no modules/${mod}.sh)"
    [[ "$mod" == "docker" ]] && needs_restart=1
  done <<< "$modules_list"

  # Per-project resource overrides; any omitted field keeps the base.yaml
  # default. Validate before yq so a bad value gives a friendly error instead of
  # an obscure yq failure, and so the unquoted cpus value can't inject yq
  # expressions (P4).
  local res_cpus res_memory res_disk
  res_cpus="$(yq -r '.resources.cpus // ""' "$config_file" 2>/dev/null || true)"
  res_memory="$(yq -r '.resources.memory // ""' "$config_file" 2>/dev/null || true)"
  res_disk="$(yq -r '.resources.disk // ""' "$config_file" 2>/dev/null || true)"
  [[ -n "$res_cpus"   && "$res_cpus"   != "null" ]] && validate_cpus "$res_cpus"
  [[ -n "$res_memory" && "$res_memory" != "null" ]] && validate_size memory "$res_memory"
  [[ -n "$res_disk"   && "$res_disk"   != "null" ]] && validate_size disk "$res_disk"

  _CREATE_TMPFILE="$(mktemp)"
  _CREATE_VM_NAME=""
  _CREATE_DONE=0
  trap cleanup_create EXIT
  cp "$REPO_DIR/base.yaml" "$_CREATE_TMPFILE"

  # Derive a valid Linux username from the host username following Lima's rules.
  local vm_user
  vm_user="$(id -un)"
  vm_user="$(printf '%s' "$vm_user" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9_-' '_')"
  [[ "$vm_user" =~ ^[a-z_] ]] || vm_user="_${vm_user}"
  vm_user="${vm_user:0:32}"

  # Resolve the guest home from Lima's default template, falling back to Lima's
  # conventional path when limactl info doesn't expose it, instead of dying (P8).
  local lima_info default_home default_user guest_home mount_point
  lima_info="$(limactl info 2>/dev/null || true)"
  default_home="$(printf '%s' "$lima_info" | yq -r '.defaultTemplate.user.home' 2>/dev/null || true)"
  default_user="$(printf '%s' "$lima_info" | yq -r '.defaultTemplate.user.name' 2>/dev/null || true)"
  if [[ -n "$default_home" && "$default_home" != "null" && -n "$default_user" && "$default_user" != "null" ]]; then
    guest_home="${default_home/$default_user/$vm_user}"
  else
    warn "could not resolve guest home from 'limactl info'; falling back to /home/${vm_user}.linux"
    guest_home="/home/${vm_user}.linux"
  fi
  mount_point="${guest_home}/${project_name}"

  yq -i ".user.name = \"$vm_user\" | .user.home = \"$guest_home\"" "$_CREATE_TMPFILE"

  yq -i ".mounts += [{
    \"location\": \"$host_project_path\",
    \"mountPoint\": \"$mount_point\",
    \"writable\": true
  }]" "$_CREATE_TMPFILE"

  [[ -n "$res_cpus"   && "$res_cpus"   != "null" ]] && yq -i ".cpus = $res_cpus" "$_CREATE_TMPFILE"
  [[ -n "$res_memory" && "$res_memory" != "null" ]] && yq -i ".memory = \"$res_memory\"" "$_CREATE_TMPFILE"
  [[ -n "$res_disk"   && "$res_disk"   != "null" ]] && yq -i ".disk = \"$res_disk\"" "$_CREATE_TMPFILE"

  info "Resources: cpus=$(yq -r '.cpus' "$_CREATE_TMPFILE") memory=$(yq -r '.memory' "$_CREATE_TMPFILE") disk=$(yq -r '.disk' "$_CREATE_TMPFILE")"

  info "Creating VM: $vm_name"
  _CREATE_VM_NAME="$vm_name"   # from here on, a failure rolls back the VM (P1)
  limactl create --name="$vm_name" --tty=false "$_CREATE_TMPFILE"

  info "Starting VM: $vm_name"
  limactl start "$vm_name"

  # Detect the actual VM user.
  local detected_user
  detected_user="$(limactl shell "$vm_name" whoami)"
  [[ -n "$detected_user" ]] || die "could not detect VM user"

  # Always run base first, then config modules in order.
  run_module "$vm_name" "$detected_user" "$project_name" "base"
  while IFS= read -r mod; do
    [[ -z "$mod" || "$mod" == "null" ]] && continue
    run_module "$vm_name" "$detected_user" "$project_name" "$mod"
  done <<< "$modules_list"

  # Modules succeeded: the VM is usable. Past this point a failure (e.g. the
  # optional restart) must NOT roll back the provisioned VM (P1).
  _CREATE_DONE=1

  # The restart only applies docker group membership; skip it otherwise (P5).
  if [[ "$needs_restart" -eq 1 ]]; then
    info "Restarting VM to apply group changes: $vm_name"
    limactl restart "$vm_name"
  fi

  info ""
  info "VM ready: $vm_name"
  info ""
  info "Connect:  vm shell $project_name"
  info "          ssh lima-$vm_name"
  info "VS Code:  Remote-SSH -> lima-$vm_name"
}
