# agent-vm

Isolated Linux development VMs for AI-assisted work on macOS, one VM per project,
via [Lima](https://lima-vm.io/). Each VM carries only the tools its project selects.
Driven by a single Go binary, `avm`.

## Prerequisites

```bash
brew install lima
```

## Install

```bash
go install github.com/MikD1/agent-vm/cmd/avm@latest
```

## Usage

### Scenario A — mount mode (code on the host)

The host project directory is mounted into the VM; you edit on the host and the VM
sees the changes. Commit/diff/branch inside the VM; push/pull on the host where
credentials live.

```bash
cd ~/projects/my-api
avm init                 # write .agent-vm.yaml, then edit it to select modules
avm create               # create + provision the VM (Record + VM)
avm shell                # open a shell at the workspace
```

### Scenario B — clone mode (code never on the host)

No host mount: the repo is cloned inside the VM, authenticated through your
forwarded SSH agent (keys stay on the host). The VM Record is the only host-side
description of the VM.

```bash
avm create --repo=git@github.com:acme/my-api.git --modules=node,claude
avm shell my-api
```

With no `--modules` and no `.agent-vm.yaml` in the repo, a default set
(`node`, `claude`) is installed.

## Commands

| Command | Description |
|---------|-------------|
| `avm init [path]` | Write a `.agent-vm.yaml` template. `--force` overwrites. |
| `avm create [path]` | Mount mode from a project dir. |
| `avm create --repo=URL` | Clone mode (`--ref`, `--modules`, `--cpus`, `--memory`, `--disk`, `--base-image`). |
| `avm recreate <name>` | Pristine rebuild from the record (clone mode re-clones — commit & push first). |
| `avm list` | List VMs: managed / orphaned / unmanaged. |
| `avm shell [name]` | Open a shell in the VM. |
| `avm start/stop/restart [name]` | Lifecycle controls. |
| `avm delete <name>` | Stop + delete the VM and remove its record. `--force` skips confirmation. |
| `avm prune [name]` | Remove orphaned records (record without a VM). |

`[name]` defaults to the current project (the `.agent-vm.yaml` directory's basename).

### Global flags

| Flag | Description |
|------|-------------|
| `--verbose` | Show the full Lima log. By default only `avm`'s own `==>` progress plus Lima warnings and errors are shown; with `--verbose` every Lima line is shown. Either way the `time=…level=…` prefix and trailing fields are stripped to plain text. Colors honor `NO_COLOR`. |

## Project config — `.agent-vm.yaml`

```yaml
modules: [node, docker, claude]
resources:
  cpus: 8        # default 4
  memory: 16GiB  # default 4GiB
  disk: 200GiB   # default 120GiB
# base: { image: corp-ubuntu }   # optional; default template:_images/ubuntu
```

## Modules

| Module | Description |
|--------|-------------|
| `node` | Node.js LTS + npm/pnpm/yarn |
| `dotnet` | .NET SDK (LTS) |
| `go` | Go toolchain (latest stable) |
| `docker` | Docker CE |
| `claude` | Claude Code CLI (needs `node`) |
| `codex` | OpenAI Codex CLI (needs `node`) |

The `base` module (git, curl, jq, ripgrep, fd, build-essential) is always installed.

### Configuring the `claude` module

The `claude` module picks up two optional files from the host secrets directory
(`~/.config/agent-vm/`, mounted read-only into the VM). Both are applied at
provision time — edit them before `avm create`, or run `avm recreate <name>` to
apply changes to an existing VM.

**Settings** — drop a [Claude Code settings file](https://docs.claude.com/en/docs/claude-code/settings)
at `~/.config/agent-vm/modules/claude/settings.json`. It is copied verbatim to
`~/.claude/settings.json` inside the VM, so it can carry permissions, environment
variables, hooks, model selection, and the like.

**Plugins** — list plugins to install, one per line, in
`~/.config/agent-vm/modules/claude/plugins`:

```
# blank lines and #-comments are ignored
some-plugin                  # bare name → official marketplace
other-plugin@my-marketplace  # name@marketplace → that marketplace
```

A bare `name` installs from the official marketplace
(`anthropics/claude-plugins-official`), which the module adds and updates for you;
`name@marketplace` targets another marketplace, which must already be registered.
Plugins install at user scope into the VM user's `~/.claude`; one that fails to
install logs a warning and provisioning continues.

### Configuring the `codex` module

The `codex` module installs the OpenAI Codex CLI (`npm install -g @openai/codex`)
and, like `claude`, needs the `node` module installed first. It picks up two
optional files from the host secrets directory (`~/.config/agent-vm/`, mounted
read-only into the VM). Both are applied at provision time — edit them before
`avm create`, or run `avm recreate <name>` to apply changes to an existing VM.

**Config** — drop a [Codex config file](https://developers.openai.com/codex/config-reference)
at `~/.config/agent-vm/modules/codex/config.toml`. It is copied verbatim to
`~/.codex/config.toml` inside the VM, so it can carry model settings, the
approval policy, and MCP servers (configured as `[mcp_servers.<id>]` tables).

**Credentials** — to authenticate non-interactively, drop the auth file at
`~/.config/agent-vm/modules/codex/auth.json` (the same JSON Codex writes when you
run `codex login`). It is copied to `~/.codex/auth.json` inside the VM with `0600`
permissions. Without it, sign in from inside the VM with `codex login`, or set
`OPENAI_API_KEY`.

## How it works

`avm` is a Go orchestrator over three layers with narrow interfaces:

1. **Go CLI** parses config, owns the registry, and plans provisioning. It is the
   only thing that reasons in domain terms.
2. **Lima** virtualizes: `avm` shells out to `limactl` (a stable CLI contract) to
   create/start/shell/delete VMs.
3. **Bash provisioning** runs inside the guest in a fixed phase sequence.

### Two config artifacts

- **Project Spec** (`.agent-vm.yaml`, in your repo) — portable *intent*: modules,
  resources, optional base image.
- **VM Record** (`~/.config/agent-vm/vms/<name>.yaml`, host-local) — the tool's
  *materialization* of one Lima VM (resolved spec + create-time facts). `avm`
  reconciles the registry against Lima on every `list` and labels each VM
  **managed**, **orphaned** (record without VM), or **unmanaged** (VM without
  record). `create` writes the record first; if provisioning fails the VM is rolled
  back and the record is kept as orphaned, recoverable via `recreate`/`prune`.

### Provisioning phases

```
Phase 0  create + start the VM from the base image
Phase 1  system layer — install host CA certs into the trust store, export trust
         env globally (modules never touch certificates)
Phase 2  base module — git, curl, jq, ripgrep, fd, build-essential, sanitized gitconfig
Phase 3  feature modules in spec order (node, dotnet, go, docker, claude)
Phase 4  workspace — mount is already present; clone runs `git clone` via the
         forwarded SSH agent
```

Each script runs as root with a small env contract: `VM_USER`, `VM_PROJECT`,
`VM_WORKSPACE`, `VM_SECRETS` (`/mnt/host/agent-vm`, read-only).

### Custom CA certificates

Drop PEM root CAs into `~/.config/agent-vm/ca-certificates/`; the Phase 1 system
layer installs them into the VM trust store and exports the trust env globally, so
node/git/python/curl all inherit it with no per-module configuration.

## Security

- Each project is isolated in its own VM.
- Secrets are mounted read-only from the host.
- Git credentials stay on the host (mount mode) or in the forwarded SSH agent
  (clone mode) — keys never leave the host.
