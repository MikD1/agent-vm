# shellcheck shell=bash
# shellcheck disable=SC2154  # REPO_DIR is exported by bin/vm
# vm init — write a commented .ai-dev-vm.yaml template.

cmd_init() {
  local force=0 target_dir="$PWD"
  for arg in "$@"; do
    case "$arg" in
      -f|--force) force=1 ;;
      -*) die "unknown option: $arg" ;;
      *) target_dir="$arg" ;;
    esac
  done

  [[ -d "$target_dir" ]] || die "directory not found: $target_dir"
  local dest="$target_dir/.ai-dev-vm.yaml"

  if [[ -e "$dest" && "$force" -ne 1 ]]; then
    die ".ai-dev-vm.yaml already exists in $target_dir (use --force to overwrite)"
  fi

  cp "$REPO_DIR/templates/ai-dev-vm.yaml" "$dest"
  info "Created $dest"
  info "Edit it to select modules, then run: vm create"
}
