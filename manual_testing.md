# Manual Integration Testing Checklist

Requires macOS with Lima installed. Run before merging `feat/go-rewrite` into `main`.

## Prerequisites

```bash
# Rename the repo on GitHub: ai-dev-vm → agent-vm, then:
git remote set-url origin https://github.com/MikD1/agent-vm.git
git checkout feat/go-rewrite
go install ./cmd/avm
```

---

## Checklist

### 1. Mount mode (basic scenario)

```bash
cd ~/projects/my-api   # project with .agent-vm.yaml
avm init               # creates .agent-vm.yaml
# edit it — add the node module
avm create
avm shell
```

Verify: VM started, modules installed, `~/.config/agent-vm/vms/my-api.yaml` created, `avm shell` opens a shell in the workspace directory.

---

### 2. Clone mode

```bash
avm create --repo=git@github.com:MikD1/agent-vm.git --modules=node,claude
avm shell agent-vm
```

Verify: repo cloned inside the VM via SSH agent, `~/.config/agent-vm/vms/agent-vm.yaml` created with `mode: clone`.

---

### 3. Default modules (clone from a bare repo — no Spec, no --modules)

```bash
avm create --repo=<any repo without .agent-vm.yaml>
```

Verify: `node` + `claude` installed in the VM (DefaultModules).

---

### 4. Recreate

```bash
avm recreate <name>
```

Verify: VM deleted and rebuilt from scratch, Record unchanged.

---

### 5. Reconciliation labels in `avm list`

```bash
# Delete VM directly, bypassing avm:
limactl delete -f my-api
avm list
# → my-api should appear as orphaned
```

```bash
# Create VM directly, bypassing avm:
limactl create --name=alien   # any template
avm list
# → alien should appear as unmanaged
```

---

### 6. Prune

```bash
avm prune my-api   # removes orphaned Record
avm list           # my-api no longer appears
```

---

### 7. Provisioning failure → OrphanedRecord

```bash
# Create a broken external module:
mkdir -p ~/.config/agent-vm/modules.d
echo '#!/usr/bin/env bash; exit 99' > ~/.config/agent-vm/modules.d/bad.sh
chmod +x ~/.config/agent-vm/modules.d/bad.sh

cd ~/projects/test-rollback
avm init && echo "modules: [bad]" > .agent-vm.yaml
avm create   # should fail at phase 3
```

Verify: VM deleted (`limactl list` shows no `test-rollback`), Record present (`~/.config/agent-vm/vms/test-rollback.yaml` exists), error message mentions `avm recreate`/`avm prune`.

---

### 8. Custom CA certificates

```bash
mkdir -p ~/.config/agent-vm/ca-certificates
cp ~/corp-root-ca.pem ~/.config/agent-vm/ca-certificates/

avm recreate my-api
avm shell my-api
```

Inside the VM:

```bash
node -e "console.log(process.env.NODE_EXTRA_CA_CERTS)"
# → /etc/ssl/certs/agent-vm-ca-bundle.pem

printenv SSL_CERT_FILE REQUESTS_CA_BUNDLE GIT_SSL_CAINFO CURL_CA_BUNDLE
# → all four variables point to the same file
```

---

### 9. Delete

```bash
avm delete <name> --force
```

Verify: VM deleted (`limactl list` does not show `<name>`), Record deleted (`ls ~/.config/agent-vm/vms/` does not contain `<name>.yaml`).

---

## After the checklist

Record results in the PR description and merge `feat/go-rewrite` into `main`.
