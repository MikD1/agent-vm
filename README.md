# AI Dev VM

Modular Linux VM environment for AI-assisted development on macOS via [Lima](https://lima-vm.io/).

Each project gets an isolated VM with only the tools it needs. Modules are selected per-project via a `.ai-dev-vm.yaml` config file, and the whole lifecycle is driven by the `vm` command.

## Prerequisites

```bash
brew install lima yq
```

## Install

From the cloned repo:

```bash
./install.sh
```

To remove the `vm` symlink that install.sh added:

```bash
./uninstall.sh
```

It never touches third-party tools (lima, yq, brew), your Lima VMs, or `~/.config/ai-dev-vm` (which may hold a customized `.gitconfig`) — it points those out so you can clean them up yourself.

## Quick Start

```bash
cd ~/projects/my-project
vm init
# edit .ai-dev-vm.yaml
vm create
vm shell
```

Run inside a project directory (one containing `.ai-dev-vm.yaml`) and `vm` uses that project automatically — the VM name is the directory's basename. You can also target any VM by name from anywhere, e.g. `vm shell my-project`. The name is normalized to a lowercase DNS label (Lima's requirement): uppercase becomes lowercase and other characters become hyphens, so a directory named `My_Project` yields the VM name `my-project`. Because the name is just the basename, two directories with the same basename (e.g. `~/work/api` and `~/personal/api`) map to the same VM; `vm create` refuses the second with an "already exists" error.

## Commands

| Command | Description |
|---------|-------------|
| `vm init [path]` | Write a commented `.ai-dev-vm.yaml` template (default: current dir). `--force` overwrites. |
| `vm create [path]` | Create + start a VM from a project dir (default: current dir). |
| `vm list` | List all Lima VMs. |
| `vm shell [name]` | Open a shell in the VM. |
| `vm start [name]` | Start a stopped VM. |
| `vm stop [name]` | Stop a running VM. |
| `vm restart [name]` | Restart a VM. |
| `vm delete [name]` | Stop and delete a VM. `--force` skips the confirmation. |

`[name]` defaults to the current project; `[path]` defaults to the current directory.

To connect from VS Code: Remote-SSH → `lima-<name>`.

Git inside the VM: commit, diff, log, branch, rebase — all local. Git on the host: push, pull, fetch — where credentials live.

To change a VM's resources or modules, re-create it: `vm delete <name>` then `vm create`.

## How It Works

```mermaid
graph TB
    subgraph Host["Host"]
        config["~/.config/ai-dev-vm/<br/>(secrets &amp; module configs)"]
        project["~/projects/my-project/<br/>.ai-dev-vm.yaml"]
        cli["vm create"]
    end

    subgraph VM["Lima VM: my-project"]
        secrets["/mnt/host/ai-dev-vm<br/><i>read-only</i>"]
        workdir["/home/user.guest/my-project<br/><i>writable</i>"]
        subgraph Modules["Module execution (as root)"]
            direction LR
            base["base.sh<br/>always first"]
            mod1["module 1"]
            mod2["module 2"]
            base --> mod1 --> mod2
        end
    end

    config -- "virtiofs mount" --> secrets
    project -- "virtiofs mount<br/>(added by vm create)" --> workdir
    cli -- "reads .ai-dev-vm.yaml" --> project
    cli -- "creates VM from base.yaml<br/>runs modules in order" --> VM
    secrets -- "$VM_SECRETS" --> Modules
```

`vm create` does the following:

1. Reads `.ai-dev-vm.yaml` from the project root to get the list of modules
2. Creates a Lima VM from `base.yaml` and adds a writable mount for the project directory
3. Runs `base.sh` (always), then each module from the config in order

Each module runs as root with these environment variables:

| Variable | Value | Description |
|----------|-------|-------------|
| `VM_USER` | auto-detected | Unprivileged user in the VM |
| `VM_PROJECT` | project name | Used in paths |
| `VM_SECRETS` | `/mnt/host/ai-dev-vm` | Read-only mount of `~/.config/ai-dev-vm` |

Module-specific configs go in `~/.config/ai-dev-vm/modules/<name>/` on the host and are accessible inside modules at `$VM_SECRETS/modules/<name>/`.

## Project Config

`.ai-dev-vm.yaml` selects modules and, optionally, VM resources:

```yaml
modules:
  - node
  - docker
  - claude
resources:
  cpus: 8        # default: 4
  memory: 16GiB  # default: 4GiB
  disk: 200GiB   # default: 120GiB
```

Each `resources` field is optional; omitted fields keep the default. Values pass straight to Lima, so use Lima's formats (a plain integer for `cpus`, a size string like `16GiB` for `memory`/`disk`).

## Modules

| Module | Description |
|--------|-------------|
| `node` | Node.js (LTS, pinned to Node 24.x via NodeSource) + npm + pnpm + yarn |
| `dotnet` | .NET SDK (latest LTS) |
| `docker` | Docker CE |
| `claude` | Claude Code CLI |

The `base` module (git, curl, jq, ripgrep, fd, build-essential) is always installed automatically.

## Custom CA Certificates

If your network uses SSL inspection (corporate proxy), place root CA certificates in PEM format into:

```bash
mkdir -p ~/.config/ai-dev-vm/ca-certificates
cp your-corp-ca.pem ~/.config/ai-dev-vm/ca-certificates/
```

`base.sh` installs them into the VM's system trust store before other modules run.

## Module Configuration

### claude

Create `~/.config/ai-dev-vm/modules/claude/settings.json` with your Claude Code settings. To supply an API key, use the `env` block:

```json
{
  "env": {
    "ANTHROPIC_API_KEY": "your-key-here"
  }
}
```

`settings.json` holds general Claude Code settings — there is no `apiKey` field. Provide the key via the `ANTHROPIC_API_KEY` environment variable (as in the `env` block above), or via `apiKeyHelper` for a command-sourced key that isn't stored in plaintext.

To pre-install plugins, create `~/.config/ai-dev-vm/modules/claude/plugins` with one plugin name per line:

```
superpowers
```

Lines starting with `#` are ignored.

## Adding a Module

Create `modules/<name>.sh` following the module contract:

```bash
#!/usr/bin/env bash
set -euo pipefail
# Runs as root, DEBIAN_FRONTEND=noninteractive
# Available env vars: VM_USER, VM_PROJECT, VM_SECRETS
```

Then use `<name>` in `.ai-dev-vm.yaml`.

Module config files (if needed) go in `~/.config/ai-dev-vm/modules/<name>/` on the host, accessible inside the module at `$VM_SECRETS/modules/<name>/`.

## Security

- Each project is isolated in its own VM
- Secrets mounted read-only from host, loaded only in subshells
- Git credentials stay on host — no duplication
- SSH agent forwarding disabled
