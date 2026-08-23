# agent-vm

Isolated Linux development VMs for AI-assisted work on macOS, one VM per domain
of work, via [Lima](https://lima-vm.io/). A VM is described by its own folder on
the host: the config lives there, along with the keys, certificates and agent
settings delivered into the guest. Driven by a single Go binary, `avm`.

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

A VM is a folder. Put its config there, along with anything the guest needs, and
list the projects it works on:

```bash
mkdir -p ~/vms/work && cd ~/vms/work
avm init                 # write agent-vm.yaml, then edit it
avm create               # create + provision the VM (Record + VM)
avm shell work           # open a shell in the VM (lands in the guest home)
```

Each project in `mounts` is mounted read/write at `~/<name>` in the guest; you
edit on the host and the VM sees the changes. Commit/diff/branch inside the VM;
push/pull on the host where credentials live.

### Mounting a project at runtime

A project can be attached to an existing VM without editing the config by hand:

```bash
cd ~/projects/web
avm mount work           # mounts the current folder into the VM named `work`
avm unmount work web     # detach it again, by guest name or host path
```

Both commands update the VM's `agent-vm.yaml`, its Record, and the VM itself.
Lima attaches folders at boot and will not rewrite a running instance's config,
so a running VM has to be stopped and started again for the change to take
effect — `avm mount` asks first. Declining leaves `agent-vm.yaml` and the Record
updated but the VM as it was; a plain `avm restart` does *not* apply them. To
apply them later, stop the VM and run the command again, or `avm recreate
<name>`. Everything inside the guest survives the stop/start.

A VM's own directory cannot be mounted into it: it is already there, read-only,
at `/mnt/host/vm`.

Two projects with the same folder name collide at `~/<name>`; pass
`--name <name>` to place one of them elsewhere.

## Commands

| Command | Description |
|---------|-------------|
| `avm init [path]` | Write an `agent-vm.yaml` template. `--force` overwrites. |
| `avm create [path]` | Create + provision the VM from a VM directory. |
| `avm recreate <name>` | Pristine rebuild from the VM's current `agent-vm.yaml` (from the record when that folder is not on this host). |
| `avm list [name]` | List VMs with registry status (managed / orphaned / unmanaged) and Lima runtime state. With a name, show that VM's mounts and tools. |
| `avm mount <vm> [path]` | Mount a project folder into a VM. `--name` overrides the guest directory name. |
| `avm unmount <vm> <path\|name>` | Detach a project folder from a VM. |
| `avm shell [name]` | Open a shell in the VM, at the guest home (`~`), with each mounted project one `cd` away. |
| `avm start/stop/restart [name]` | Lifecycle controls. |
| `avm delete <name>` | Stop + delete the VM and remove its record. `--force` skips confirmation. |
| `avm prune [name]` | Remove orphaned records (record without a VM). |

`[name]` defaults to the VM whose directory you are in (from `name:` in
`agent-vm.yaml`, or the folder's basename). `avm mount` and `avm unmount` always
take the name explicitly: they run from a project folder, not from the VM's.

### Global flags

| Flag | Description |
|------|-------------|
| `--verbose` | Show the full Lima log. By default only `avm`'s own `==>` progress plus Lima warnings and errors are shown; with `--verbose` every Lima line is shown. Either way the `time=…level=…` prefix and trailing fields are stripped to plain text. Colors honor `NO_COLOR`. |

## VM config — `agent-vm.yaml`

```yaml
name: work           # optional; defaults to this folder's basename

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
  codex-auth.json:
    to: ~/.codex/auth.json
    mode: "0600"

scripts:             # bash, run last, as root
  - provision/postgres.sh

mounts:              # the projects this VM works on
  - ~/projects/api   #   → ~/api in the guest
  - ~/projects/web   #   → ~/web
  - path: ~/other/api
    name: api-legacy #   → ~/api-legacy
```

Mount paths must be absolute or start with `~/`, so a VM's contents never depend
on where its folder happens to sit. Each mounts read/write at `~/<name>`, where
`name` defaults to the folder's basename.

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
`build-essential`, Docker, and mise itself. The `.gitconfig` is read from
`<vm-dir>/.gitconfig` if that file exists — put the VM's git identity there, per
VM, next to `agent-vm.yaml`; every `credential.*` section is stripped on the way
in, which is what makes it sanitized.

## Config files

Tool configuration and credentials are declared in `files`, as host source →
guest destination. Sources are relative to `agent-vm.yaml`, or absolute, and must
sit inside the VM directory: the copy runs inside the guest, and the VM directory
is the only host folder a `files` source can be read from. Keeping them there is
also what keeps them out of any project repository.

```yaml
files:
  claude-settings.json: ~/.claude/settings.json
  codex-config.toml: ~/.codex/config.toml
  tmux.conf: ~/.tmux.conf
  codex-auth.json:
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
with `VM_USER`, `VM_HOME`, and `VM_CONFIG` set, and a non-zero exit fails
provisioning.

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
   (one `mise install`) → config files → user scripts → restart. Only the tools
   phase depends on the project; everything else is the same on every VM.

### Two config artifacts

- **VM Spec** (`agent-vm.yaml`, in the VM's folder) — portable *intent*: modules,
  resources, mounted projects, files, scripts, optional base image.
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
Phase 3  tools — one `mise install` for every module in the spec, retried up
         to three times so a stalled download does not cost the whole VM
Phase 4  config files — copy each `files` entry from the VM directory to its
         guest destination
Phase 5  user scripts — run each `scripts` entry, in order, as root
Phase 6  restart — applies group membership (docker) and anything else a live
         session holds
```

Each script runs as root with a small env contract: `VM_USER`, `VM_HOME`,
`VM_CONFIG` (`/mnt/host/vm`, the VM directory, read-only).

A phase is quiet while it runs: `avm` buffers the guest's stdout and streams only
its stderr. Phase 2 downloads mise from GitHub's release CDN — a host none of the
earlier phases touch — so a network path that blocks or throttles it fails there
and nowhere else. That download takes the compressed archive (~22 MB, not the
~95 MB bare binary), is bounded (connect timeout, stall guard, retries), resumes
a partial file instead of restarting it, and prints its progress every 20s. A
failure names the URL to measure by hand.

### Custom CA certificates

Drop PEM root CAs into `<vm-dir>/ca-certificates/`; the Phase 1 system layer
installs them into the VM trust store and exports the trust env globally, so
node/git/python/curl all inherit it with no per-tool configuration.

## Security

- Each domain of work is isolated in its own VM.
- The VM directory is mounted read-only; the guest cannot write back to it.
- Git credentials stay on the host — they never leave it.
