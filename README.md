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
into `~/.local/bin` by default and checks for `limactl`. Creating a VM requires
Lima 2.1.0 or newer. If `limactl` is missing and Homebrew is available, the
installer runs `brew install lima`. Otherwise it gives instructions for
installing Homebrew or Lima manually.

Set `AVM_INSTALL_DIR` to choose the install directory, or `AVM_VERSION` to pin a
release tag.

```bash
curl -fsSL https://raw.githubusercontent.com/MikD1/agent-vm/main/install.sh |
  AVM_INSTALL_DIR="$HOME/bin" AVM_VERSION=v.0.10 sh
```

The installer finds the latest version through github.com's own
`/releases/latest` redirect, so it needs no call to `api.github.com` — a host
corporate proxies often block outright, and one GitHub rate-limits at 60
requests/hour for an unauthenticated IP. Either produces a `403` on a release
that is public and downloads fine. The API is still tried as a fallback, and if
both are unreachable the installer prints what each host answered; pinning
`AVM_VERSION` skips release discovery altogether.

### Install from source

Use Go 1.26.4 or newer. Run the second command from a local checkout.

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

Choose a VM name unused in Lima. If Lima returns a creation error, `avm create`
attempts to delete the instance with that name as part of cleanup.

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

Both commands update `agent-vm.yaml` and the Record before syncing the full
mount list to Lima. For a stopped VM the new mounts take effect on the next
start. For a running VM both commands ask before stopping, editing, and starting
it. This stop/start preserves the VM's disk.

Declining saves the requested change in the Spec and Record only. An ordinary
`avm restart` uses the existing Lima configuration. Repeating the same mount
or unmount exits before syncing. The CLI message points to
`avm recreate <name>`, which applies the saved Spec by rebuilding the VM and
discarding guest-only state.

The VM directory is already available read-only at `/mnt/host/vm`. Project
mounts targeting that directory or a directory that contains it are rejected.

Two projects with the same folder name collide at `~/<name>`; pass
`--name <name>` to place one of them elsewhere.

## Commands

| Command | Description |
|---------|-------------|
| `avm init [path]` | Write an `agent-vm.yaml` template. `--force` overwrites. |
| `avm create [path]` | Create + provision the VM from a VM directory. |
| `avm recreate <name>` | Rebuild from the current VM Spec, using Record parameters if the Spec is absent. Required host paths must still be available. Guest-only state is discarded. |
| `avm list [name]` | List all VMs with registry status and Lima runtime state. With a name, show that VM's recorded mounts and tools. |
| `avm mount <vm> [path]` | Mount a project folder into a VM. `--name` overrides the guest directory name. |
| `avm unmount <vm> <path\|name>` | Detach a project folder from a VM. |
| `avm shell [name]` | Open a shell in the VM at the recorded guest home, or let Lima choose when the home is unavailable. |
| `avm start/stop/restart [name]` | Lifecycle controls. |
| `avm delete <name>` | Stop + delete the VM and remove its record. `--force` skips confirmation. |
| `avm prune [name]` | Remove a named orphaned Record, or all orphaned Records when the name is omitted. |

Only `shell`, `start`, `stop`, and `restart` infer an omitted name from the
current directory. These commands use `name:` from its `agent-vm.yaml` or the
directory's basename when the value is empty or omitted. `mount` and `unmount`
require the VM name.

### Global flags

| Flag | Description |
|------|-------------|
| `--verbose` | Include informational and debug lines from Lima's structured stderr and enable verbose mise installation. Warnings, errors, and unstructured guest stderr are shown by default. Structured lines are rendered as plain text and colors honor `NO_COLOR`. The interactive `avm shell` passes the terminal streams directly to Lima. |

### Create flags

`--cpus`, `--memory`, `--disk`, `--base-image`, and `--modules` override
values from the Spec for that creation. For example, `--modules=node@lts,go@1.24`
selects the tools to install. A later `avm recreate` with an available Spec
resolves it again, using built-in defaults for omitted settings.

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
  - node: lts        # track the Node.js LTS alias
  - go: "1.24"       # track the Go 1.24 series
  - claude           # no version → the latest release
  - python: "3.12"
```

Omit `modules` to get `node@lts` and `claude@latest`. Use `modules: []` to
install only the platform packages. The file generated by `avm init` explicitly
selects Node LTS, Claude, and Codex.

Tools install system-wide under `/usr/local/share/mise`, and their shim directory
is on `PATH` in login shells, in the shell `avm shell` opens, and under `sudo`.
The VM user owns the tool directory and can run `mise upgrade <tool>` inside the
VM without `sudo`. New releases are eligible immediately within the configured
version range. `avm` records the versions mise resolved during provisioning in
the VM Record.

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

The destination may be absolute or start with `~/`. For regular files, `mode`
defaults to `0644`. Set it to `0600` for files holding credentials. Directory
sources are copied recursively with `cp -R`, so modes of newly created entries
can be affected by the guest's umask. The `mode` option is only for file sources.

Files are copied at provision time. Edit them and run `avm recreate <name>` to
apply changes to an existing VM.

## Provisioning scripts

For anything neither a tool nor a file — a database, a service, a corporate
setup step — list bash scripts in `scripts`. Each path is absolute on the host
or relative to the Spec directory. The scripts run in order as root before the
final restart, with `VM_USER`, `VM_HOME`, and `VM_CONFIG` set. A non-zero exit
fails provisioning.

```yaml
scripts:
  - provision/postgres.sh
```

To make an installed tool write its configuration to the VM user's home, run it
in that user's login shell.

```bash
sudo -u "$VM_USER" -H bash -lc 'claude plugin install some-plugin'
```

## How it works

`avm` is a Go orchestrator over three layers with narrow interfaces:

1. **Go CLI** parses config, owns the registry, and plans provisioning. It is the
   only thing that reasons in domain terms.
2. **Lima** virtualizes: `avm` shells out to `limactl` (a stable CLI contract) to
   create/start/shell/delete VMs.
3. **Bash provisioning** configures trust and the platform, installs tools,
   copies config files, runs user scripts, and restarts the guest. Tools, file
   copies, and user scripts come from the resolved VM configuration.

### Two config artifacts

- **VM Spec** (`agent-vm.yaml`, in the VM's folder) — portable *intent*: modules,
  resources, mounted projects, files, scripts, optional base image.
- **VM Record** (host-local, `~/.config/agent-vm/vms/<name>.yaml` by default)
  captures the resolved Spec and create-time facts. If set, `XDG_CONFIG_HOME`
  replaces `~/.config`. Each `list` matches entries by name and labels them
  **managed** (VM and Record), **orphaned** (Record only), or **unmanaged**
  (VM only). `create` writes the Record first and keeps it if provisioning
  fails. Cleanup of the Lima VM is attempted separately.

### Provisioning phases

```
Phase 0  create + start the VM from the base image
Phase 1  system layer — install host CA certs into the trust store, export trust
         env globally (tools never touch certificates)
Phase 2  platform — apt packages, Docker, mise itself; always installed, never
         selected by a project
Phase 3  tools — run `mise install` for the full module list as the VM user
         with up to three attempts total, then create shims and read versions
Phase 4  config files — copy each `files` entry from the VM directory to its
         guest destination
Phase 5  user scripts — run each `scripts` entry, in order, as root
Phase 6  restart — applies group membership (docker) and anything else a live
         session holds
```

The provisioning wrapper starts each phase as root and sets `VM_USER`,
`VM_HOME`, and `VM_CONFIG` (`/mnt/host/vm`, the read-only VM directory).
Phase 3 runs mise as `VM_USER`.

`avm` buffers the guest's stdout and streams its stderr. When installing mise
itself in Phase 2, it downloads a compressed release from GitHub's release CDN
with connection and stall limits and retry support. The download can resume a
partial file and reports progress about every 20 seconds. A failure names the
download URL.

### Custom CA certificates

Drop the root CAs into `<vm-dir>/ca-certificates/`; the Phase 1 system layer
installs them into the VM trust store and exports the trust env globally, so
node/git/python/curl/mise all inherit it with no per-tool configuration.

Phase 1 skips hidden files and files ending in `.md`. It accepts PEM and DER
regardless of the filename extension. A corporate root CA exported from a
Windows trust store as `corp-root.crt` works as-is. Put the whole chain in if the
proxy presents one (root plus any subordinate CAs), one file per certificate or
several in one PEM.

On macOS, a CA already trusted by the host can be exported straight out of the
keychain:

```bash
mkdir -p ~/vms/work/ca-certificates
security find-certificate -a -c "<certificate name>" -p \
  /Library/Keychains/System.keychain > ~/vms/work/ca-certificates/corp-root.pem
```

Phase 1 prints one `system: trusting …` line per accepted input file with the
first certificate's subject when available. It also prints the file count and
warns about rejected candidates. When `curl` is available, it tries HTTPS
requests to `github.com` and `api.github.com`. Failures warn and provisioning
continues.

If Phase 3 reports `invalid peer certificate: UnknownIssuer` on a
TLS-inspecting network, check whether the VM trusts the CA your network uses
to sign certificates. From a working guest, check verification for a
specific host with `openssl s_client -connect api.github.com:443
-verify_hostname api.github.com -verify_return_error < /dev/null`.
Add the required CAs to the VM directory and rebuild to apply them.

## Security

- Each domain of work is isolated in its own VM.
- The service mount at `/mnt/host/vm` is read-only. Entries in `mounts` are
  read/write.
- Push and pull from the host to keep Git credentials there. The base phase
  strips credential sections from the provided `.gitconfig`. Entries in
  `files` explicitly copy their contents into the guest.
