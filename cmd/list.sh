# shellcheck shell=bash
# vm list — list all Lima VMs (not just ai-dev-vm ones).

cmd_list() {
  preflight
  limactl list "$@"
}
