# shellcheck shell=bash
# shellcheck disable=SC2154  # REPO_DIR is exported by bin/vm
# vm create — create and start a Lima VM for a project.

cmd_create() {
  preflight

  local host_project_path="${1:-$PWD}"
  [[ -d "$host_project_path" ]] || die "project directory not found: $host_project_path"
  host_project_path="$(cd "$host_project_path" && pwd)"

  local project_name vm_name
  project_name="$(basename "$host_project_path")"
  validate_name "$project_name"
  vm_name="$project_name"

  local config_file="$host_project_path/.ai-dev-vm.yaml"
  [[ -f "$config_file" ]] || die ".ai-dev-vm.yaml not found in $host_project_path (run: vm init)"

  if vm_exists "$vm_name"; then
    die "VM '$vm_name' already exists. Delete it first: vm delete $project_name"
  fi

  _CREATE_TMPFILE="$(mktemp)"
  trap 'rm -f "$_CREATE_TMPFILE"' EXIT
  cp "$REPO_DIR/base.yaml" "$_CREATE_TMPFILE"

  # Derive a valid Linux username from the host username following Lima's rules.
  local vm_user
  vm_user="$(id -un)"
  vm_user="$(printf '%s' "$vm_user" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9_-' '_')"
  [[ "$vm_user" =~ ^[a-z_] ]] || vm_user="_${vm_user}"
  vm_user="${vm_user:0:32}"

  local lima_info default_home default_user guest_home mount_point
  lima_info="$(limactl info)"
  default_home="$(printf '%s' "$lima_info" | yq -r '.defaultTemplate.user.home')"
  default_user="$(printf '%s' "$lima_info" | yq -r '.defaultTemplate.user.name')"
  [[ -n "$default_home" && "$default_home" != "null" ]] || die "could not resolve guest home directory from limactl info"
  [[ -n "$default_user" && "$default_user" != "null" ]] || die "could not resolve guest username from limactl info"
  guest_home="${default_home/$default_user/$vm_user}"
  mount_point="${guest_home}/${project_name}"

  yq -i ".user.name = \"$vm_user\" | .user.home = \"$guest_home\"" "$_CREATE_TMPFILE"

  yq -i ".mounts += [{
    \"location\": \"$host_project_path\",
    \"mountPoint\": \"$mount_point\",
    \"writable\": true
  }]" "$_CREATE_TMPFILE"

  # Per-project resource overrides; any field omitted keeps the base.yaml default.
  local res_cpus res_memory res_disk
  res_cpus="$(yq -r '.resources.cpus // ""' "$config_file" 2>/dev/null || true)"
  res_memory="$(yq -r '.resources.memory // ""' "$config_file" 2>/dev/null || true)"
  res_disk="$(yq -r '.resources.disk // ""' "$config_file" 2>/dev/null || true)"
  [[ -n "$res_cpus" && "$res_cpus" != "null" ]] && yq -i ".cpus = $res_cpus" "$_CREATE_TMPFILE"
  [[ -n "$res_memory" && "$res_memory" != "null" ]] && yq -i ".memory = \"$res_memory\"" "$_CREATE_TMPFILE"
  [[ -n "$res_disk" && "$res_disk" != "null" ]] && yq -i ".disk = \"$res_disk\"" "$_CREATE_TMPFILE"

  info "Resources: cpus=$(yq -r '.cpus' "$_CREATE_TMPFILE") memory=$(yq -r '.memory' "$_CREATE_TMPFILE") disk=$(yq -r '.disk' "$_CREATE_TMPFILE")"

  info "Creating VM: $vm_name"
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
  done <<< "$(yq -r '.modules[]' "$config_file" 2>/dev/null || true)"

  info "Restarting VM to apply group changes: $vm_name"
  limactl restart "$vm_name"

  info ""
  info "VM ready: $vm_name"
  info ""
  info "Connect:  vm shell $project_name"
  info "          ssh lima-$vm_name"
  info "VS Code:  Remote-SSH -> lima-$vm_name"
}
