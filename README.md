# AI Dev VM

Modular Linux VM environment for AI-assisted development on macOS via [Lima](https://lima-vm.io/).

Each project gets an isolated VM with only the tools it needs. Modules are selected per-project via a `.ai-dev-vm.yaml` config file.

## Prerequisites

```bash
brew install lima yq
```

## Setup

Create a local secrets directory (not committed to git):

```bash
mkdir -p ~/.config/ai-dev/certs
chmod 700 ~/.config/ai-dev
cp ~/.gitconfig ~/.config/ai-dev/.gitconfig
```

Create `~/.config/ai-dev/secrets.env` with your API keys:

```bash
ANTHROPIC_API_KEY=your-key-here
```

```bash
chmod 600 ~/.config/ai-dev/secrets.env
```

## Usage

### 1. Add config to your project

Create `.ai-dev-vm.yaml` in the project root:

```yaml
modules:
  - node
  - docker
  - claude
```

### 2. Create a VM

```bash
./scripts/create-vm.sh my-project
# or with explicit path:
./scripts/create-vm.sh my-project ~/Work/my-project
```

### 3. Connect

```bash
limactl shell dev-my-project
# or
ssh lima-dev-my-project
```

VS Code: Remote-SSH → `lima-dev-my-project`

### 4. Work

```bash
cd ~/my-project
claude-env              # Claude Code with secrets loaded
```

Git inside VM: commit, diff, log, branch, rebase — all local operations.
Git on host: push, pull, fetch — where credentials are configured.

### 5. Delete when done

```bash
./scripts/delete-vm.sh my-project
```

### Update

Re-create instead of updating:

```bash
./scripts/delete-vm.sh my-project
./scripts/create-vm.sh my-project
```

## Available Modules

| Module | Description |
|--------|-------------|
| `node` | Node.js (latest LTS) + npm + pnpm + yarn |
| `dotnet` | .NET SDK (latest LTS) |
| `docker` | Docker CE |
| `claude` | Claude Code CLI + `claude-env` wrapper |

`base` module (git, curl, jq, ripgrep, fd, build-essential) is always installed automatically.

## Adding a Module

Create `modules/<name>.sh` following the module contract:

```bash
#!/usr/bin/env bash
set -euo pipefail
# Runs as root, DEBIAN_FRONTEND=noninteractive
# Available env vars: VM_USER, VM_PROJECT, VM_SECRETS
```

Then use `<name>` in `.ai-dev-vm.yaml`.

## Security

- Each project is isolated in its own VM
- Secrets mounted read-only from host, loaded only in subshells
- Git credentials stay on macOS — no duplication
- SSH agent forwarding disabled
