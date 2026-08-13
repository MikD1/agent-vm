# agent-vm

Isolated Linux development VMs for AI-assisted work on macOS, one VM per project,
via [Lima](https://lima-vm.io/). Each VM carries only the tools its project selects.
Driven by a single Go binary, `avm`.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/MikD1/agent-vm/main/install.sh | sh
```

The installer supports macOS. It installs `avm` from the latest GitHub Release
and checks for Lima, the only runtime dependency. If Lima is missing and
Homebrew is already installed, the script runs `brew install lima`; if Homebrew
is also missing, it exits with instructions for installing Homebrew or Lima
manually. The installer does not install Homebrew itself.

Set `AVM_INSTALL_DIR` to choose the install directory, or `AVM_VERSION` to pin a
release tag:

```bash
curl -fsSL https://raw.githubusercontent.com/MikD1/agent-vm/main/install.sh | AVM_INSTALL_DIR=/usr/local/bin AVM_VERSION=v0.1.0 sh
```

### Install from source

For development, or when you already have Go installed:

```bash
go install github.com/MikD1/agent-vm/cmd/avm@latest
go install ./cmd/avm
```

## Usage

The host project directory is mounted into the VM; you edit on the host and the VM
sees the changes. Commit/diff/branch inside the VM; push/pull on the host where
credentials live.

```bash
cd ~/projects/my-api
avm init                 # write .agent-vm.yaml, then edit it to select modules
avm create               # create + provision the VM (Record + VM)
avm shell                # open a shell at the workspace
```

### Additional mounts — projects spanning multiple folders

Beyond the primary project directory, a VM can mount extra host folders —
useful when a project spans a main repo plus sibling libraries, utilities, or
tools. Declare them in `.agent-vm.yaml` (paths relative to the file, see
[Project config](#project-config--agent-vmyaml)) and/or pass `--mount PATH`
(repeatable) at `avm create` time; flags add to the spec list, they don't
replace it:

```bash
avm create --mount ../shared-lib --mount ../tools/cli
```

Each folder mounts read/write at `~/<basename>` in the guest, or at `~/<name>`
if you set an explicit `name:` in the spec to resolve a basename collision.
Additional mounts are always writable, need no provisioning of their own — Lima
mounts everything at VM start — and are stored in the VM Record, so
`avm recreate` reproduces them automatically without passing `--mount` again.

## Commands

| Command | Description |
|---------|-------------|
| `avm init [path]` | Write a `.agent-vm.yaml` template. `--force` overwrites. |
| `avm create [path]` | Create + provision the VM from a project dir. Add `--mount PATH` (repeatable) to mount extra host folders. |
| `avm recreate <name>` | Pristine rebuild from the record. |
| `avm list` | List VMs with registry status (managed / orphaned / unmanaged) and Lima runtime state (running / stopped). |
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
modules:             # mise tools; a name with no version installs the latest
  - node: lts
  - go: "1.24"
  - claude

resources:
  cpus: 8            # default 4
  memory: 16GiB      # default 4GiB
  disk: 200GiB       # default 120GiB

# base: { image: corp-ubuntu }   # optional; default template:_images/ubuntu

files:               # host source → guest destination
  claude-settings.json: ~/.claude/settings.json
  ~/.config/agent-vm/codex-auth.json:
    to: ~/.codex/auth.json
    mode: "0600"

scripts:             # bash, run last, as root
  - provision/postgres.sh

mounts:              # extra host folders, relative to this file
  - ../shared-lib    #   → mounted read/write at ~/shared-lib
```

## Modules

A module is a [mise](https://mise.jdx.dev/) tool: a name, optionally with a
version. Anything in mise's registry works, as do backend references such as
`npm:@openai/codex` or `aqua:owner/repo` — `avm` needs no change to support a new
tool.

```yaml
modules:
  - node: lts        # a pinned alias
  - go: "1.24"       # a pinned version
  - claude           # no version → the latest release
  - python: "3.12"
```

Tools install system-wide under `/usr/local/share/mise`, and their shim directory
is on `PATH` in login shells, in the shell `avm shell` opens, and under `sudo`.
`avm` records the versions mise actually resolved in the VM Record.

Every VM also gets, without being asked: the host CA certificates in the system
trust store, a sanitized `.gitconfig`, `git`, `curl`, `jq`, `ripgrep`, `fd`,
`build-essential`, Docker, and mise itself.

## Config files

Tool configuration and credentials are declared in `files`, as host source →
guest destination. Sources are relative to `.agent-vm.yaml`, or absolute, and
must sit either next to it or under `~/.config/agent-vm/` — the two directories
the VM can see. Keep credentials in `~/.config/agent-vm/` so they stay out of the
repository.

```yaml
files:
  claude-settings.json: ~/.claude/settings.json
  codex-config.toml: ~/.codex/config.toml
  tmux.conf: ~/.tmux.conf
  ~/.config/agent-vm/codex-auth.json:
    to: ~/.codex/auth.json
    mode: "0600"
```

The destination may be absolute or start with `~/`. `mode` defaults to `0644`;
set it to `0600` for anything holding credentials. A directory source is copied
recursively and keeps its own permissions, so `mode` is not allowed on one.

Files are copied at provision time. Edit them and run `avm recreate <name>` to
apply changes to an existing VM.

## Provisioning scripts

For anything neither a tool nor a file — a database, a service, a corporate
setup step — list bash scripts in `scripts`. They run last, in order, as root,
with `VM_USER`, `VM_PROJECT`, `VM_WORKSPACE`, and `VM_SECRETS` set, and a
non-zero exit fails provisioning.

```yaml
scripts:
  - provision/postgres.sh
```

Root's `PATH` carries no mise shims during provisioning, so to use an installed
tool, drop to the VM user's login shell:

```bash
sudo -u "$VM_USER" -H bash -lc 'claude plugin install some-plugin'
```

## How it works

`avm` is a Go orchestrator over three layers with narrow interfaces:

1. **Go CLI** parses config, owns the registry, and plans provisioning. It is the
   only thing that reasons in domain terms.
2. **Lima** virtualizes: `avm` shells out to `limactl` (a stable CLI contract) to
   create/start/shell/delete VMs.
3. **Bash provisioning** runs inside the guest in a fixed phase sequence: system
   layer (certificates, trust) → platform (apt packages, Docker, mise) → tools
   (one `mise install`) → workspace → config files → user scripts → restart. Only
   the tools phase depends on the project; everything else is the same on every VM.

### Two config artifacts

- **Project Spec** (`.agent-vm.yaml`, in your repo) — portable *intent*: modules,
  resources, files, scripts, optional base image.
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
         env globally (tools never touch certificates)
Phase 2  platform — apt packages, Docker, mise itself; always installed, never
         selected by a project
Phase 3  tools — one `mise install` for every module in the spec
Phase 4  workspace — mount is already present via virtiofs
Phase 5  config files — copy each `files` entry from the host mount to its guest
         destination
Phase 6  user scripts — run each `scripts` entry, in order, as root
Phase 7  restart — applies group membership (docker) and anything else a live
         session holds
```

Each script runs as root with a small env contract: `VM_USER`, `VM_PROJECT`,
`VM_WORKSPACE`, `VM_SECRETS` (`/mnt/host/agent-vm`, read-only).

### Custom CA certificates

Drop PEM root CAs into `~/.config/agent-vm/ca-certificates/`; the Phase 1 system
layer installs them into the VM trust store and exports the trust env globally, so
node/git/python/curl all inherit it with no per-tool configuration.

## Security

- Each project is isolated in its own VM.
- Secrets are mounted read-only from the host.
- Git credentials stay on the host — they never leave it.
