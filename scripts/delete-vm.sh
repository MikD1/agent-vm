#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "Usage: $0 <project-name>"
  echo "Example: $0 my-api"
  exit 1
fi

PROJECT_NAME="$1"
VM_NAME="$PROJECT_NAME"

if ! limactl list --format '{{.Name}}' 2>/dev/null | grep -qx "$VM_NAME"; then
  echo "VM '$VM_NAME' does not exist."
  exit 0
fi

echo "Stopping VM: $VM_NAME"
limactl stop "$VM_NAME" 2>/dev/null || true

echo "Deleting VM: $VM_NAME"
limactl delete -f "$VM_NAME"

echo "Deleted: $VM_NAME"
