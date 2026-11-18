# AGENTS.md

Guidance for AI coding agents (Claude, Codex, opencode, …) working on **agent-vm**.
Read this before making changes. It is the canonical contract for how to build, test,
modify, and ship work in this repo. Human-facing usage docs live in
[`README.md`](README.md); the design rationale lives in [`docs/architecture.md`](docs/architecture.md).

> Instruction priority: this file and any user request override default agent
> behavior. Where a tool/skill conflicts with the conventions here, follow this file.

---

## 1. What this project is

`agent-vm` is a single Go CLI, **`avm`**, that provisions isolated Linux development
VMs on macOS via [Lima](https://lima-vm.io/) — one VM per domain of work, each carrying
only the tools that domain selects. It exists so AI agents can work in throwaway,
isolated environments without polluting the host.

A VM is described by its own folder on the host: `agent-vm.yaml` lives there,
together with everything delivered into the guest — agent configs, keys,
certificates, provisioning scripts.

- **Module path:** `github.com/MikD1/agent-vm`
- **Binary:** `avm` (entrypoint `cmd/avm/main.go`)
- **Language:** Go **1.26.4** (pinned in `go.mod`)
- **Direct deps:** `github.com/spf13/cobra` (CLI), `gopkg.in/yaml.v3` (config). Keep the
  dependency set minimal — Lima is the only intended *runtime* dependency.
- **License:** MIT. Do not add license headers to source files.
- **Platform:** macOS host with Lima 2.1.0 or newer. Keep the Lima and virtiofs assumptions.

### The three-layer model (memorize this)

```
Go CLI (internal/*)  →  shells out to `limactl`  →  bash scripts provision the guest
   orchestrates             virtualizes                 set up platform and tools
```

Each layer talks to the next through a narrow, stable interface. Most bugs and bad PRs
come from blurring these boundaries — don't.

---

## 2. Setup, build, test, lint

The [`Makefile`](Makefile) is the source of truth for commands. Use the Go toolchain
version in `go.mod` and make ShellCheck available on `PATH` for shell lint. Builds
and tests run without Lima. See §6 for the Lima test fakes.

| Task | Command | Notes |
|------|---------|-------|
| Build the binary | `make build` | → `bin/avm` from `./cmd/avm`. `bin/` is gitignored. |
| Compile-check all packages | `go build ./...` | No binary emitted. |
| Run all tests | `make test` (`go test ./...`) | Tests live only under `internal/*`; `cmd/avm` has none. |
| Run one package | `go test ./internal/cli` | |
| Run one test | `go test ./internal/cli -run TestRootHasVerbosePersistentFlag` | `-run` is a regex; add `-v` for verbose. |
| Vet | `make vet` (`go vet ./...`) | |
| Format | `make fmt` (`gofmt -w cmd internal`) | Rewrites in place. |
| Check formatting | `make fmt-check` | Fails, listing the offending files, if `gofmt -l cmd internal` is non-empty. |
| Shellcheck installer and guest scripts | `make shellcheck` | `shellcheck install.sh internal/provision/scripts/*.sh`. |
| Lint (fmt-check + vet + shellcheck) | `make lint` | |
| **Default gate** | `make all` | Runs **fmt-check → vet → test → build only**. |

### Critical tooling facts

- **`make all`** runs `fmt-check → vet → test → build`. Run `make shellcheck`
  (or `make lint`) as well when editing `install.sh` or a guest script.
- **CI runs the same targets** (`.github/workflows/ci.yml`: `make fmt-check`, `make vet`,
  `make build`, `make test`, plus `make shellcheck` in its own job; `release.yml` runs all
  of them). There is no `CONTRIBUTING.md`. CI is a second net, not a substitute: run the
  local gates before every PR.
- **"Lint" means exactly `gofmt` + `go vet` + `shellcheck`.** There is no `golangci-lint`
  and no `.golangci*` config. Do not introduce one or assume linters that aren't here.
- **The tree is `gofmt`-clean and stays that way.** `make fmt-check` (part of `make all`
  and of CI) fails on any unformatted file under `cmd/` or `internal/`; `make fmt` fixes
  it. Never commit a formatting-only change to a file your work does not otherwise
  touch — it buries the real diff.

---

## 3. Repository layout

```
cmd/avm/main.go                 entrypoint: calls cli.Execute(), then prints error + os.Exit(1)
internal/
  cli/        cobra commands (one file per command) — the only package that wires everything
  config/     VM Spec schema, defaults, validation, flags>spec>defaults resolution
  specedit/   comment-preserving edits to agent-vm.yaml (yaml.Node, not re-marshal)
  registry/   VM Records (default ~/.config/agent-vm/vms/) + Lima reconciliation
  lima/       limactl wrapper — the ONLY package that shells out to limactl
  provision/  phase planner + go:embed of scripts/*.sh — drives the bash phases via lima;
              the ONLY package that knows the platform-script layout. Per-tool
              installation is NOT a script here — it is one rendered `mise install`
              built from the resolved config's Modules ([]config.ModuleSpec).
  vmname/     VM name normalization / validation
  templates/  go:embed of the Lima base template + the agent-vm.yaml init template
internal/provision/scripts/*.sh   platform provisioning bash (embedded into the binary):
                                   system, base, docker, mise — never per-tool scripts
internal/templates/files/*        base.yaml (Lima) + agent-vm.yaml (Spec template)
```

Import direction (a DAG — never create a cycle): `cmd/avm → cli`; `cli` is the wiring
layer and fans out to `{config, specedit, registry, provision, lima, templates, vmname}`;
`specedit → config`; `provision → {config, lima}`; `registry → config`. `internal/config` has
**zero** internal dependencies (pure domain types).

---

## 4. Code conventions & architecture rules (Go)

### Hard constraints — do not violate

1. **Only `internal/lima` may invoke `limactl`.** All Lima access goes through
   `lima.Client`, which is driven by the injectable `lima.CommandRunner` interface.
   Never write `exec.Command("limactl", …)` outside `internal/lima`. (Mentioning
   "limactl" in a comment or user-facing error string is fine.)
2. **Only `internal/provision` knows the platform-script layout.** The
   `//go:embed scripts/*.sh` directive lives there (`internal/provision/scripts.go`).
   Other packages never read `scripts/` themselves; the unexported `guestScript(name)`
   accessor is the only way in, and it stays internal to the package — callers outside
   `internal/provision` don't request platform scripts by name at all, they just call
   `provision.New(...).Run(...)`. Per-tool installation is not a script and has no
   equivalent accessor: a tool is a `config.ModuleSpec` (name + version) resolved by
   mise at provision time, never embedded.
3. **No upward or cyclic imports.** `internal/config` must stay dependency-free.
   `Spec.Validate()` takes no argument — module names are mise tool references and are
   not checked against a catalog (an unknown name surfaces when mise runs), so there is
   nothing to inject. `config` must never import `registry`, `lima`, or `provision`.

### The config types (Spec → Resolved → Record)

- **`config.Spec`** (`internal/config/spec.go`) — the portable, human-authored VM
  Spec (`agent-vm.yaml`): `Name`, `Modules *[]ModuleSpec`, `Resources`, `Base`,
  `Mounts`, `Files`, `Scripts`. `Modules` is a **pointer on purpose**: `nil` (key
  absent → defaults may apply) is semantically different from an explicit empty
  list (no tools beyond the always-installed platform). Do not "simplify" it to
  `[]ModuleSpec`. Each `ModuleSpec` is a mise tool reference (name + optional
  version) accepting either a bare scalar (`- claude`) or a single-key mapping
  (`- node: lts`) in YAML, or `name@version` (`node@lts`) on the `--modules` flag
  via `ParseModuleRef`. `Mounts` entries are absolute or `~/`-prefixed host paths
  — never relative, so a VM's contents do not depend on where its folder sits.
  The absolute path of the folder itself is a create-time fact recorded as
  `ConfigDir` in the Record.
- **`config.Resolved`** (`internal/config/resolve.go`) — the bridge produced by
  `config.Resolve`. Both the Lima config and the Record are built from it.
- **`registry.Record`** (`internal/registry/record.go`) — the host-local materialization
  of one VM (resolved spec + create-time facts: `CreatedAt`, `User`, `ConfigDir`, `Home`).
  Build it **only** via `registry.FromResolved(resolved, now)`; never hand-construct a
  Record or duplicate resolution logic.

### Config resolution order

Precedence is **flags > in-repo `agent-vm.yaml` > built-in defaults**, implemented
entirely in `config.Resolve` using the `firstInt`/`firstStr` helpers plus the
`Default*` constants in `internal/config/defaults.go`. To make an unset flag not shadow
the spec, a `*Set bool` companion is wired from cobra's `cmd.Flags().Changed("…")`
(see `Flags.ModulesSet`, set in `internal/cli/create.go`). Mirror this pattern for any
new "absent vs explicit" flag.

### Error handling

- Wrap with `fmt.Errorf("context: %w", err)` at each layer boundary; add a short context
  prefix (phase name, `read spec`, `parse record %q`).
- **No package-level sentinel errors.** `errors.Is` is used only against stdlib errors
  (`os.ErrNotExist`). Don't add `var ErrFoo = errors.New(…)` unless a caller genuinely
  needs to branch on it.
- Drift/status is modeled with typed string constants (`registry.Status`:
  `managed`/`orphaned`/`unmanaged`), not errors.

### Cobra wiring

- One root factory `NewRootCmd()` (`internal/cli/root.go`) registers every subcommand via
  `root.AddCommand(…)` and sets `SilenceUsage: true` + `SilenceErrors: true`. The single
  error print + `os.Exit(1)` lives in `cmd/avm/main.go` — **do not** add `fmt.Println` +
  `os.Exit` inside command handlers.
- Each command is a `newXxxCmd() *cobra.Command` factory (one per file, except the
  lifecycle verbs `shell`/`start`/`stop`/`restart` which share `internal/cli/lifecycle.go`),
  using **`RunE`** so errors propagate. Commands that need Lima build the client
  inside `RunE` via `newLimaClient(cmd)`, which honors the persistent `--verbose`
  flag. Get `ctx` from `cmd.Context()`.
- **Keep logic out of the closure:** `RunE` parses flags/derives inputs, then calls a
  testable plain function (`runCreate`, `runList`, `runDelete`, …) that takes injected
  dependencies. This is what makes the command unit-testable without a real `limactl`.

### Naming & docs

- Every package has a doc comment on its `package` clause stating its single
  responsibility. Add one for any new package.
- Exported identifiers are minimal and documented; helpers stay unexported.
- Code must pass `gofmt`, `go vet ./...`, `go build ./...`, and `go test ./...`.

---

## 5. Provisioning scripts (bash) conventions

Guest provisioning lives in `internal/provision/scripts/*.sh`, embedded into the binary.
These are **platform scripts**, not per-tool scripts — there is no script per module.
The full phase sequence, driven from `Provisioner.Run` (`internal/provision/provision.go`):

1. **Phase 0** — create + start the VM (Lima).
2. **Phase 1** runs `system.sh` to pick up PEM and DER CA files from
   `$VM_CONFIG/ca-certificates` and export trust settings. It skips hidden and
   `.md` files and reports accepted and rejected candidates on stderr. HTTPS
   preflight failures produce warnings when `curl` is available.
3. **Phase 2** always installs the platform using `base.sh`
   (apt packages, sanitized gitconfig), `docker.sh`, `mise.sh` (installs mise itself).
4. **Phase 3** writes the system mise config, sets `minimum_release_age = "0s"`,
   and assigns the data directory to `VM_USER`. Each of up to three attempts
   runs one generated `mise install` for the full resolved module list as the VM
   user. The same user runs `mise reshim` and `mise ls -i -J` to create shims and
   read versions for the Record's `installedTools`. A read-back failure warns
   and provisioning continues.
5. **Phase 4** — config files: copy each `files` entry from `$VM_CONFIG` to its destination.
6. **Phase 5** — user scripts, in spec order (arbitrary bash a project supplies via
   `scripts:` in `agent-vm.yaml` — not to be confused with the embedded platform
   scripts this section is about).
7. **Phase 6** — restart, unconditionally.

Adding support for a new *tool* needs no script at all — see §7.A. This section's rules
apply to the four embedded **platform** scripts (`system.sh`, `base.sh`, `docker.sh`,
`mise.sh`) and to any new one you add at that layer.

### How scripts actually run

Scripts are **piped to `bash -s` over stdin as root** — not executed as files. The Go
provisioner runs `limactl shell --workdir / <vm> sudo bash …` and pipes the script bytes
in, re-exporting the contract vars from positional args and setting
`DEBIAN_FRONTEND=noninteractive` and `bash -euo pipefail`. So the shebang/`+x` bit are
not what makes a script run — but keep the header anyway (required for `make shellcheck`
and for running a script directly while debugging).

Mandatory header for every script:

```bash
#!/usr/bin/env bash
set -euo pipefail
```

The Go wrapper already exports `DEBIAN_FRONTEND=noninteractive` globally before running
the script, so adding `export DEBIAN_FRONTEND=noninteractive` in-script is optional and
redundant (about half the existing scripts include it for clarity; both styles are fine).

### Guest env contract (the only inputs a platform script or a `files`/`scripts` entry may rely on)

| Variable | Value | Use |
|----------|-------|-----|
| `VM_USER` | unprivileged guest user | `runuser`, `sudo -u "$VM_USER" -H …`, `usermod` |
| `VM_HOME` | the guest home | the root every project mounts under |
| `VM_CONFIG` | `/mnt/host/vm` (**read-only** virtiofs) | the VM directory: `files` sources, `ca-certificates/`, and the sanitized `.gitconfig` `base.sh` reads |

Keep VM directory inputs under `$VM_CONFIG` in the guest. The CLI reads the
configured project scripts on the host and streams them over stdin. Keep the
three-variable interface for passing VM paths and user information.

### Rules for platform scripts

- **Keep certificate management in `system.sh`.** Phase 1 installs host CAs and
  publishes trust settings. `provisionWrapper` sources them when present, and
  `runuser` preserves them for mise. Later platform phases and tools inherit
  these settings. Keep CA installation, `ca-certificates/` reads, and trust
  variable assignments centralized.
  Use the merged system bundle for variables that replace the roots so public
  roots stay trusted. Phase 1 currently falls back to the host-only bundle when
  the merged bundle is absent or empty. `SSL_CERT_DIR` points to
  `/etc/ssl/certs`, and `NODE_EXTRA_CA_CERTS` points to the host-only bundle
  because Node adds those certificates to its built-in roots.
- **Be idempotent** (scripts re-run on `avm recreate`): install-guard with
  `command -v <tool> >/dev/null 2>&1 || { install… }`; edit env files delete-then-append
  per key, not blind append; use `ln -sf` for PATH shims.
- **PATH needs two places.** `avm shell` opens a **non-login** shell that does not source
  `/etc/profile.d`. For a tool to be on `PATH` there, add a `/usr/local/bin` symlink or
  an `/etc/environment` entry **in addition to** `/etc/profile.d/<name>.sh` (see how
  `mise.sh` wires mise's shim dir into both `/etc/profile.d/mise.sh` and
  `/etc/environment`, plus `secure_path` in a sudoers drop-in for `sudo <tool>`).
- **Big downloads go to `/var/tmp`, not `/tmp`** (`/tmp` is a small tmpfs that overflows
  on SDK/toolchain archives — `mise.sh` downloads the mise release binary there).
- **Failure handling:** use `|| true` only for genuinely optional side effects (e.g.
  docker `systemctl enable`/`usermod`). On the hard-requirement path, do not swallow
  exit codes — let `set -e` fail, or echo `Warning: …` to stderr for best-effort steps.
- **Must pass `shellcheck` with no inline disable directives** (there is no
  `.shellcheckrc`).

---

## 6. Testing conventions

Tests use the **standard library `testing` only** — no testify, no mocking library, no
golden files, no `testdata/`. Keep it that way.

- **Every behavioral change ships with a test**, in a `*_test.go` in the same package.
  Match the style of the package you're editing.
- **Faking `limactl` — the one seam.** The `lima.CommandRunner` interface
  (`Run(ctx, stdin []byte, args ...string) (stdout, stderr []byte, err error)`) is the
  only place to fake the VM layer. Production uses `lima.ExecRunner`; tests hand-write a
  small struct implementing `Run` and pass it to `lima.New(fakeRunner)`. Do not invent a
  generic mock — write a purpose-built fake (existing patterns: `recorder`/`okRunner`
  record the subcommand sequence; `fakeRunner` returns canned stdout keyed by joined
  args; `failRunner` forces a failure on a chosen subcommand; `namesRunner` returns a
  fixed `limactl list`). Never shell out to a real `limactl` in a test.
- **Dependency injection by plain struct/params.** Functions under test take their
  collaborators as arguments (`runCreate(ctx, createDeps{lima: lima.New(fr), store: …})`),
  so tests pass fakes directly. There is no global state to reset.
- **Filesystem isolation.** Use `t.TempDir()` for all on-disk state and
  `registry.NewStore(t.TempDir())` for the registry. Never write to the real registry
  root or `$HOME`.
- **Determinism.** Use a fixed clock (a `nowFixed()`-style constant `time.Time`) instead
  of `time.Now()` when a `Record.CreatedAt` or similar is asserted.
- **Style.** Table-driven (`tests := []struct{…}` + `for _, tt := range tests {
  t.Run(tt.name, …) }`) when there are many cases; a flat case slice is fine for a few.
  Mark helpers with `t.Helper()`. Assertions are inline (equality, `strings.Contains`,
  joined-slice compare) — no golden files.
- **Ordering tests are exact.** Provisioning/lifecycle tests assert the precise sequence
  of `limactl` subcommands (`args[0]` per call). If you change the create/start/provision/
  restart pipeline, update the expected slices in `internal/provision/provision_test.go`
  and the relevant `internal/cli/*_test.go`.

`go test ./...` must stay green.

---

## 7. How to make common changes

Three layers move: Go (`internal/`), bash (`internal/provision/scripts/`), templates
(`internal/templates/files/`). After **any** change: run `make all`; if you touched a
`.sh`, also run `make shellcheck`. Always add/extend a test and update docs
(`README.md` + `docs/architecture.md` are part of "done").

### A. Add support for a new tool (e.g. `rust`, `python`) vs. add a new platform script

These are two different things — don't confuse them.

**Adding a tool needs no code change at all**, if it's in mise's registry (or reachable
via a mise backend like `npm:`/`aqua:`/`cargo:`). A user just adds it to `modules:` in
their `agent-vm.yaml` (or `--modules=name@version` on `avm create`) — Phase 3 renders it
into the one `mise install` invocation. There is nothing to embed, register, or test in
this repo for that case; if a request describes "adding a new module," check first
whether it's really just a mise tool reference the user can add to their own Spec.

**Adding a new *platform* script** (a Phase-1/2 step like `system.sh`/`base.sh`/
`docker.sh`/`mise.sh` — something that must run as root, once, for every VM, regardless
of the project's tool selection) is the actual analog of the old "add a module" recipe:

1. Create `internal/provision/scripts/<name>.sh`. Copy an existing platform script as a
   template. Follow §5: header, env contract, idempotency, no CA handling (except in
   `system.sh` itself).
2. Wire it into the phase sequence in `internal/provision/provision.go` — most likely
   into the Phase 2 platform loop (`{"base", …}, {"docker", …}, {"mise", …}`), in the
   order it must run.
3. If it needs a **VM restart** to take effect (currently `docker`, for group
   membership, and the unconditional Phase 6 restart already covers this — a new
   platform step should not need its own restart logic).
4. Extend `internal/provision/scripts_test.go`'s `TestGuestScriptsPresent` name list so
   the new script's shebang/`set -euo pipefail` header is checked.
5. Docs: update the phase list in `docs/architecture.md` §1/§4 and, if user-visible,
   `README.md`.

There is no `modules.d` runtime-override directory and no external-module discovery —
the platform scripts are a small, fixed, embedded set; nothing here is user-extensible
without a rebuild.

### B. Add a CLI command

Canonical small examples: `internal/cli/list.go`, `internal/cli/delete.go`.

1. Create `internal/cli/<name>.go` with **two functions**: a testable core
   `run<Name>(ctx, c *lima.Client, store *registry.Store, …)` holding the logic with
   injected deps; and `new<Name>Cmd() *cobra.Command` wiring `Use`/`Short`/`Args`/`RunE`
   and obtaining deps inside `RunE`. To follow the shell and lifecycle target
   rule, use `resolveTargetName(arg, cwd())`. It prefers the explicit argument
   and otherwise resolves the current directory's Spec name or basename. Use
   `lifecycleCmd` for commands with the same lifecycle argument handling.
2. **Register it** in `internal/cli/root.go` with `root.AddCommand(new<Name>Cmd())`.
   *(This is silent if omitted — the package compiles and tests pass while the command is
   simply absent from the binary.)*
3. Add `internal/cli/<name>_test.go` for the `run<Name>` core and/or the wiring.
4. Docs: add rows to the Commands table in `README.md` and the Command Surface table in
   `docs/architecture.md` §9.

### C. Add a VM Spec field

A field flows: YAML → `Spec` → validated → `Resolve` (flags>spec>default) → `Resolved` →
written into the `Record` and/or the Lima config. Touch each stage:

1. `internal/config/spec.go` — add the field to `Spec` (or to the `Resources`/`Base`
   sub-struct) with a `yaml:"…,omitempty"` tag. Use a pointer if "absent" must differ
   from "zero".
2. `internal/config/defaults.go` — add a `Default<Field>` if it has a built-in default.
3. `internal/config/resolve.go` — add it to `Resolved`; apply precedence in `Resolve`
   (`flag > spec > default`); extend `Validate` with any format/range check. Add a `Flags`
   field (+ `*Set bool` if absent-vs-empty matters) if it should be CLI-overridable.
4. `internal/cli/create.go` — register the cobra flag and, for absent-vs-set semantics,
   set `f.<Field>Set = cmd.Flags().Changed("<flag>")`.
5. **`internal/registry/record.go`** holds the fields that persist in the registry.
   Map them in `FromResolved`. If a field also participates in the Record
   fallback for `recreate`, map it back in `recordToResolved`.
6. `internal/cli/limaconfig.go` — if the field changes the VM itself, wire it into
   `buildLimaConfig`.
7. `internal/templates/files/agent-vm.yaml` — surface it (often commented) for `avm init`.
8. Tests: `internal/config/spec_test.go` (parse), `resolve_test.go` (precedence +
   validation), registry tests if persisted.
9. Docs: update the `agent-vm.yaml` example in `README.md` and `docs/architecture.md` §3.

---

## 8. Commits & pull requests

### Commit messages — Conventional Commits with a scope

```
type(scope): imperative subject
```

- Subject is lowercase, imperative ("add", "fix", "route" — not "added"), no trailing period.
- **Types in use:** `feat`, `fix`, `docs`, `chore`, `refactor`. (No `test:`/`ci:` types —
  ship `*_test.go` inside the `feat`/`fix` commit it belongs to.)
- **Scope** is the package or area touched — pick the one matching your change. Current
  ones: `cli`, `lima`, `provision`, `registry`, `config`, `templates`, `vmname`,
  `architecture`, and occasionally a finer command scope (`create`). A comma list
  (`cli,provision`) is fine only for a genuine cross-layer change; omit the scope only
  for repo-wide chores/docs. (Before the mise-based rewrite, per-tool bash lived under
  `internal/modules/scripts/` and commits used `modules` or a per-tool scope like
  `modules/codex` — that architecture is gone; use `provision` for platform-script
  changes now.)

Examples from history:
- `feat(registry): record the tool versions mise resolved`
- `feat(provision): scripts section for user provisioning`
- `docs(architecture): sync with implemented design`

### PR workflow

Remote is `https://github.com/MikD1/agent-vm.git`; default branch is `main`.

1. **Branch from `main`** — never commit directly to `main`:
   `git switch -c feat/<short-slug>` (or `fix/…`).
2. Make one focused change **with tests** in the same commit.
3. Run the gates (below) and confirm they pass.
4. Commit with a Conventional-Commit subject.
5. `git push -u origin <branch>`.
6. Open the PR against `main` with `gh pr create --base main --fill` or use the
   GitHub web UI. Use the same `type(scope): subject` convention for the PR title.

### Pre-PR checklist (copy-paste)

```bash
# 1. Go gate: fmt-check + vet + test + build
make all

# 2. Shell lint if you touched install.sh or a guest script
make shellcheck

# 3. If step 1 failed on fmt-check, format and re-run it
make fmt

# 4. Review your branch
git status
git log --oneline origin/main..HEAD
```

- **Do not commit build artifacts.** `bin/` and `*.test` are gitignored; there is
  precedent for an accidental binary commit being reverted.
- **Do not commit secrets.** `local/`, `*.local.yaml`, `*.env`, `*.pem`, `*.key`, `*.crt`
  are gitignored — keep CA certs, auth files, and module configs out of the repo.

---

## 9. Security & isolation notes

- Each domain of work runs in its own VM. The generated Lima configuration
  mounts the VM directory **read-only** at `/mnt/host/vm` and project folders
  read/write.
- Commit, diff, and branch inside the VM. Push and pull on the host where the
  Git credentials live. The base phase strips credential sections from the
  supplied `.gitconfig`. Entries in `files` explicitly copy their contents to
  the guest.
- The certificate model is centralized in Phase 1 (`system.sh`); modules stay unaware of
  trust (§5). Don't reintroduce per-module cert handling.

---

## 10. Non-goals (don't propose these)

The design deliberately excludes: building/baking images from within the tool
(`base.image` consumes an already-prepared image); re-applying modules to a running VM
without recreating it (the model is *change config → `avm recreate`*); importing
externally-created VMs into the registry; and non-macOS hosts. If a request seems to need
one of these, flag the tension rather than silently expanding scope.
