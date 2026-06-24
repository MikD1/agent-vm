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
VMs on macOS via [Lima](https://lima-vm.io/) — one VM per project, each carrying only
the tools its project selects. It exists so AI agents can work in throwaway, isolated
environments without polluting the host.

- **Module path:** `github.com/MikD1/agent-vm`
- **Binary:** `avm` (entrypoint `cmd/avm/main.go`)
- **Language:** Go **1.26.4** (pinned in `go.mod`)
- **Direct deps:** `github.com/spf13/cobra` (CLI), `gopkg.in/yaml.v3` (config). Keep the
  dependency set minimal — Lima is the only intended *runtime* dependency.
- **License:** MIT. Do not add license headers to source files.
- **Platform:** macOS host only (Lima/virtiofs assumptions). Do not "port" it.

### The three-layer model (memorize this)

```
Go CLI (internal/*)  →  shells out to `limactl`  →  bash scripts provision the guest
   orchestrates             virtualizes                 install tools as root
```

Each layer talks to the next through a narrow, stable interface. Most bugs and bad PRs
come from blurring these boundaries — don't.

---

## 2. Setup, build, test, lint

The [`Makefile`](Makefile) is the source of truth for commands. In this environment Go
1.26.4 and **shellcheck 0.10.0** are installed; `limactl` is **not** needed to build or
test (the Lima layer is faked in tests — see §6).

| Task | Command | Notes |
|------|---------|-------|
| Build the binary | `make build` | → `bin/avm` from `./cmd/avm`. `bin/` is gitignored. |
| Compile-check all packages | `go build ./...` | No binary emitted. |
| Run all tests | `make test` (`go test ./...`) | Tests live only under `internal/*`; `cmd/avm` has none. |
| Run one package | `go test ./internal/cli` | |
| Run one test | `go test ./internal/cli -run TestRootHasVerbosePersistentFlag` | `-run` is a regex; add `-v` for verbose. |
| Vet | `make vet` (`go vet ./...`) | |
| Shellcheck guest scripts | `make shellcheck` | `shellcheck internal/modules/scripts/*.sh`. |
| Lint (vet + shellcheck) | `make lint` | |
| **Default gate** | `make all` | Runs **vet → test → build only**. |

### Critical tooling facts

- **`make all` does NOT run shellcheck.** It is `vet → test → build`. If you edit any
  `internal/modules/scripts/*.sh`, you must additionally run `make shellcheck`
  (or `make lint`).
- **There is no CI and no `CONTRIBUTING.md`.** No `.github/` workflow runs these checks
  on push. The local gates are the *only* safety net — running them before every PR is
  mandatory, not optional.
- **"Lint" means exactly `go vet` + `shellcheck`.** There is no `golangci-lint` and no
  `.golangci*` config. Do not introduce one or assume linters that aren't here.
- **`gofmt` caveat:** two existing files — `internal/cli/target_test.go` and
  `internal/lima/logfilter_test.go` — are already flagged by `gofmt -l` (column
  alignment of one-line bodies). A blanket `gofmt -l .` is therefore non-empty on a
  clean checkout. Format only files you touch; do not reformat those two unless you're
  already editing them.

---

## 3. Repository layout

```
cmd/avm/main.go                 entrypoint: calls cli.Execute(), then prints error + os.Exit(1)
internal/
  cli/        cobra commands (one file per command) — the only package that wires everything
  config/     Project Spec schema, defaults, validation, flags>spec>defaults resolution
  registry/   VM Records (host store ~/.config/agent-vm/vms/) + Lima reconciliation
  lima/       limactl wrapper — the ONLY package that shells out to limactl
  provision/  phase planner + module runner (drives the bash phases via lima)
  modules/    go:embed of scripts/*.sh + external module discovery — the ONLY package
              that knows the script layout
  vmname/     VM name normalization / validation
  templates/  go:embed of the Lima base template + the .agent-vm.yaml init template
internal/modules/scripts/*.sh   guest provisioning bash (embedded into the binary)
internal/templates/files/*      base.yaml (Lima) + agent-vm.yaml (Spec template)
```

Import direction (a DAG — never create a cycle): `cmd/avm → cli`; `cli` is the wiring
layer and fans out to `{config, registry, provision, lima, modules, templates, vmname}`;
`provision → {config, lima, modules}`; `registry → config`. `internal/config` has
**zero** internal dependencies (pure domain types).

---

## 4. Code conventions & architecture rules (Go)

### Hard constraints — do not violate

1. **Only `internal/lima` may invoke `limactl`.** All Lima access goes through
   `lima.Client`, which is driven by the injectable `lima.CommandRunner` interface.
   Never write `exec.Command("limactl", …)` outside `internal/lima`. (Mentioning
   "limactl" in a comment or user-facing error string is fine.)
2. **Only `internal/modules` knows the bash-script layout.** The `//go:embed scripts/*.sh`
   directive and the external-dir override live there. Other packages request scripts by
   name via `modules.Script` / `modules.Exists` / `modules.List` — they never read
   `scripts/` themselves.
3. **No upward or cyclic imports.** `internal/config` must stay dependency-free.
   Validation takes an injected `known func(string) bool` callback rather than importing
   `internal/modules`. `config` must never import `registry`, `lima`, or `modules`.

### The config types (Spec → Resolved → Record)

- **`config.Spec`** (`internal/config/spec.go`) — the portable, human-authored Project
  Spec (`.agent-vm.yaml`): `Modules *[]string`, `Resources`, `Base`. `Modules` is a
  **pointer on purpose**: `nil` (key absent → defaults may apply) is semantically
  different from an explicit empty list (base-only). Do not "simplify" it to `[]string`.
  The Spec carries **no** workspace mode — that's decided by the presence of `--repo` at
  create time and recorded only in the Record.
- **`config.Resolved`** (`internal/config/resolve.go`) — the bridge produced by
  `config.Resolve`. Both the Lima config and the Record are built from it.
- **`registry.Record`** (`internal/registry/record.go`) — the host-local materialization
  of one VM (resolved spec + create-time facts: `Source`, `CreatedAt`, `User`,
  `Workspace`). Build it **only** via `registry.FromResolved(resolved, now)`; never
  hand-construct a Record or duplicate resolution logic.

### Config resolution order

Precedence is **flags > in-repo `.agent-vm.yaml` > built-in defaults**, implemented
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
  using **`RunE`** (never `Run`) so errors propagate. Build the Lima client inside `RunE` via
  `newLimaClient(cmd)` (honors the persistent `--verbose` flag); get `ctx` from
  `cmd.Context()`.
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

Guest provisioning lives in `internal/modules/scripts/*.sh`, embedded into the binary.
One script per module: `system.sh` (Phase 1), `base.sh` (Phase 2), then feature modules
`node`, `dotnet`, `go`, `docker`, `claude`, `codex` (Phase 3).

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

### Guest env contract (the only inputs a module may rely on)

| Variable | Value | Use |
|----------|-------|-----|
| `VM_USER` | unprivileged guest user | `sudo -u "$VM_USER" -H …`, `usermod` |
| `VM_PROJECT` | project / VM name | labels, naming |
| `VM_WORKSPACE` | absolute path to code in the guest | mount point or clone dir |
| `VM_SECRETS` | `/mnt/host/agent-vm` (**read-only** virtiofs) | module config at `$VM_SECRETS/modules/<name>/` |

Do not invent new contract vars and do not read anything outside `$VM_SECRETS`.

### Rules for module scripts

- **Never touch CA certificates.** Phase 1 (`system.sh`) installs host CAs and exports
  trust env (`NODE_EXTRA_CA_CERTS`, `SSL_CERT_FILE`, `REQUESTS_CA_BUNDLE`,
  `GIT_SSL_CAINFO`, `CURL_CA_BUNDLE`) globally. A feature module MUST NOT read
  `ca-certificates/`, set any `*_CA_*` var, or call `update-ca-certificates`. Trust is
  inherited transparently.
- **Module config is optional.** Read per-module config from `$VM_SECRETS/modules/<name>/`,
  always guarded with `[ -f … ]`, and degrade gracefully if absent. Credentials written
  into the user's HOME get `chmod 600`; drop privileges with `sudo -u "$VM_USER" -H …`.
- **Be idempotent** (scripts re-run on `avm recreate`): install-guard with
  `command -v <tool> >/dev/null 2>&1 || { install… }`; edit env files delete-then-append
  per key, not blind append; use `ln -sf` for PATH shims.
- **PATH needs two places.** `avm shell` opens a **non-login** shell that does not source
  `/etc/profile.d`. For a tool to be on `PATH` there, add a `/usr/local/bin` symlink or
  an `/etc/environment` entry **in addition to** `/etc/profile.d/<name>.sh` (see `go.sh`,
  `dotnet.sh`).
- **Big downloads go to `/var/tmp`, not `/tmp`** (`/tmp` is a small tmpfs that overflows
  on SDK/toolchain archives).
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

Three layers move: Go (`internal/`), bash (`internal/modules/scripts/`), templates
(`internal/templates/files/`). After **any** change: run `make all`; if you touched a
`.sh`, also run `make shellcheck`. Always add/extend a test and update docs
(`README.md` + `docs/architecture.md` are part of "done").

### A. Add a feature module (e.g. `rust`, `python`)

Modules are **discovered, not registered** — there is no allowlist or enum to edit.
`//go:embed scripts/*.sh` makes `modules.List()`/`Script()` pick up any new file
automatically.

1. Create `internal/modules/scripts/<name>.sh`. Copy an existing one (`codex.sh` is a
   good small template). Follow §5: header, env contract, idempotency, no CA handling,
   user-level installs via `sudo -u "$VM_USER" -H …`, optional config from
   `$VM_SECRETS/modules/<name>/`.
2. **Dependencies are not validated in Go.** If your module needs another (e.g. `node`),
   add a runtime guard in the script — mirror `codex.sh`
   (`command -v npm >/dev/null 2>&1 || { echo "Error: …"; exit 1; }`). Install order is
   spec order (`internal/provision/provision.go`), so the user must list the dependency
   first.
3. If your module needs a **VM restart** to take effect (currently only `docker`, for
   group membership), add a name check in `internal/provision/provision.go` to set
   `needsRestart = true`.
4. Extend the hardcoded module set in `internal/modules/modules_test.go` (the test won't
   fail if you forget — that's the trap — but it documents the module set).
5. Docs: add a row to the Modules table in `README.md`, note it in
   `docs/architecture.md` if relevant, and add a commented line to
   `internal/templates/files/agent-vm.yaml` if it's common enough for `avm init`.

> A user can also drop `<root>/modules.d/<name>.sh` at runtime (where `<root>` is
> `$XDG_CONFIG_HOME/agent-vm` if set, else `~/.config/agent-vm`), which overrides the
> embedded copy without a rebuild. The module must still appear in the resolved module
> list (flags/spec/defaults) to actually run — `modules.d` does not auto-enable a module.

### B. Add a CLI command

Canonical small examples: `internal/cli/list.go`, `internal/cli/delete.go`.

1. Create `internal/cli/<name>.go` with **two functions**: a testable core
   `run<Name>(ctx, c *lima.Client, store *registry.Store, …)` holding the logic with
   injected deps; and `new<Name>Cmd() *cobra.Command` wiring `Use`/`Short`/`Args`/`RunE`
   and obtaining deps inside `RunE`. For VM-targeting commands, resolve the name with
   `resolveTargetName(arg, cwd())` (explicit arg > basename of the cwd that contains a
   `.agent-vm.yaml` > error).
   Lifecycle-style verbs can reuse the `lifecycleCmd` helper.
2. **Register it** in `internal/cli/root.go` with `root.AddCommand(new<Name>Cmd())`.
   *(This is silent if omitted — the package compiles and tests pass while the command is
   simply absent from the binary.)*
3. Add `internal/cli/<name>_test.go` for the `run<Name>` core and/or the wiring.
4. Docs: add rows to the Commands table in `README.md` and the Command Surface table in
   `docs/architecture.md` §9.

### C. Add a Project Spec field

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
5. **`internal/registry/record.go` — if the field must persist for `recreate`/`list`,
   add it to `Record` and map it in `FromResolved`.** This is the most-missed step: skip
   it and the field resolves on create but is silently lost on `avm recreate`.
6. `internal/cli/limaconfig.go` — if the field changes the VM itself, wire it into
   `buildLimaConfig`.
7. `internal/templates/files/agent-vm.yaml` — surface it (often commented) for `avm init`.
8. Tests: `internal/config/spec_test.go` (parse), `resolve_test.go` (precedence +
   validation), registry tests if persisted.
9. Docs: update the `.agent-vm.yaml` example in `README.md` and `docs/architecture.md` §3.

---

## 8. Commits & pull requests

### Commit messages — Conventional Commits with a scope

```
type(scope): imperative subject
```

- Subject is lowercase, imperative ("add", "fix", "route" — not "added"), no trailing period.
- **Types in use:** `feat`, `fix`, `docs`, `chore`, `refactor`. (No `test:`/`ci:` types —
  ship `*_test.go` inside the `feat`/`fix` commit it belongs to.)
- **Scope** is the package, module, or area touched — pick the one matching your change.
  Common ones in history: `cli`, `lima`, `provision`, `registry`, `config`, `templates`,
  `modules`, `vmname`, `architecture`, the per-module names (`claude`, `codex`, `node`,
  `dotnet`, `go`, `base`), and occasionally a finer command scope (`create`). Use a nested
  scope like `modules/codex` for a single module; a comma list (`cli,provision`) only for
  a genuine cross-layer change; omit the scope only for repo-wide chores/docs.

Examples from history:
- `feat(cli): avm create (mount + clone, Record-first, rollback to OrphanedRecord)`
- `fix(modules): add -H to sudo in codex.sh for explicit HOME handling`
- `docs(architecture): sync with implemented design`

### PR workflow

Remote is `https://github.com/MikD1/agent-vm.git`; default branch is `main`.

1. **Branch from `main`** — never commit directly to `main`:
   `git switch -c feat/<short-slug>` (or `fix/…`).
2. Make one focused change **with tests** in the same commit.
3. Run the gates (below) and confirm they pass.
4. Commit with a Conventional-Commit subject.
5. `git push -u origin <branch>`.
6. Open the PR against `main`. **`gh` is not installed here** — either install it and run
   `gh pr create --base main --fill`, or push and open the PR via the GitHub web UI. The
   PR title follows the same `type(scope): subject` convention.

### Pre-PR checklist (copy-paste)

```bash
# 1. Go gate: vet + test + build
make all

# 2. Shell lint — ONLY if you touched internal/modules/scripts/*.sh
#    (make all does NOT run shellcheck)
make shellcheck

# 3. Format only files you changed (do not reformat the two pre-flagged test files)
gofmt -l <files-you-edited>

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

- Each project runs in its own VM; secrets are mounted **read-only** from the host at
  `/mnt/host/agent-vm`.
- Git credentials stay on the host: mount mode keeps push/pull on the host; clone mode
  uses **SSH agent forwarding enabled per clone-mode VM only** — keys never leave the host.
- The certificate model is centralized in Phase 1 (`system.sh`); modules stay unaware of
  trust (§5). Don't reintroduce per-module cert handling.

---

## 10. Non-goals (don't propose these)

The design deliberately excludes: building/baking images from within the tool
(`base.image` consumes an already-prepared image); re-applying modules to a running VM
without recreating it (the model is *change config → `avm recreate`*); importing
externally-created VMs into the registry; and non-macOS hosts. If a request seems to need
one of these, flag the tension rather than silently expanding scope.
