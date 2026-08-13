// Package provision drives the fixed phase sequence (create/start → system →
// platform → tools → files → scripts → restart) via a lima.Client.
package provision

import (
	"context"
	"fmt"
	"os"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/lima"
)

// Provisioner runs the phases for one VM.
type Provisioner struct {
	lima      *lima.Client
	verbose   bool // mirrors --verbose onto the tools the guest runs
	installed []config.ModuleSpec
}

// New builds a Provisioner.
func New(c *lima.Client, verbose bool) *Provisioner {
	return &Provisioner{lima: c, verbose: verbose}
}

// InstalledTools reports the tool versions mise actually resolved. It is valid
// after a successful Run.
func (p *Provisioner) InstalledTools() []config.ModuleSpec { return p.installed }

func (p *Provisioner) env(r config.Resolved) map[string]string {
	return map[string]string{
		"VM_USER":      r.User,
		"VM_PROJECT":   r.Name,
		"VM_WORKSPACE": r.Workspace.GuestPath,
		"VM_SECRETS":   "/mnt/host/agent-vm",
	}
}

// platform runs one embedded platform script. MISE_VERSION is prepended for the
// bootstrap rather than threaded through lima.Provision's fixed VM_* contract.
func (p *Provisioner) platform(ctx context.Context, r config.Resolved, name string) error {
	script, err := guestScript(name)
	if err != nil {
		return err
	}
	if name == "mise" {
		script = append([]byte(fmt.Sprintf("export MISE_VERSION=%s\n", miseVersion)), script...)
	}
	return p.lima.Provision(ctx, r.Name, script, p.env(r))
}

// Run executes the full sequence. The caller (cli create) handles VM rollback on
// any returned error.
func (p *Provisioner) Run(ctx context.Context, r config.Resolved, limaConfigPath string) error {
	// Phase 0 — create + start.
	fmt.Printf("==> Creating VM: %s\n", r.Name)
	if err := p.lima.Create(ctx, r.Name, limaConfigPath); err != nil {
		return err
	}
	fmt.Printf("==> Starting VM: %s\n", r.Name)
	if err := p.lima.Start(ctx, r.Name); err != nil {
		return err
	}
	// Phase 1 — system layer. Must precede everything that downloads: it is what
	// installs the host CA certificates into the trust store.
	fmt.Printf("==> Phase 1: system layer (CA certificates, trust env)\n")
	if err := p.platform(ctx, r, "system"); err != nil {
		return fmt.Errorf("phase 1 (system): %w", err)
	}
	// Phase 2 — platform. Always installed, never selected by a project.
	for _, s := range []struct{ name, label string }{
		{"base", "base packages (git, curl, build tools)"},
		{"docker", "Docker"},
		{"mise", "mise"},
	} {
		fmt.Printf("==> Phase 2: %s\n", s.label)
		if err := p.platform(ctx, r, s.name); err != nil {
			return fmt.Errorf("phase 2 (%s): %w", s.name, err)
		}
	}
	// Phase 3 — tools, in one mise invocation.
	fmt.Printf("==> Phase 3: installing %d tool(s) with mise\n", len(r.Modules))
	if err := p.lima.Provision(ctx, r.Name, renderMiseInstall(r.Modules, p.verbose), p.env(r)); err != nil {
		return fmt.Errorf("phase 3 (mise install): %w", err)
	}
	// The read-back below is metadata only: mise install (above) already
	// succeeded, so a failure here must not roll back an otherwise-successfully
	// provisioned VM. Warn and continue with p.installed left empty — the same
	// reasoning cli/create.go already applies one layer up to the Record write
	// itself ("a failure here leaves a working VM with a slightly stale Record,
	// which is not worth rolling back").
	listed, err := p.lima.ProvisionOutput(ctx, r.Name,
		[]byte("export MISE_DATA_DIR="+miseDataDir+"\nmise ls -i -J\n"), p.env(r))
	if err != nil {
		fmt.Printf("Warning: could not read back installed tool versions: %v\n", err)
	} else if p.installed, err = parseMiseList(listed); err != nil {
		fmt.Printf("Warning: could not read back installed tool versions: %v\n", err)
		p.installed = nil
	}
	// Phase 4 — configuration files, copied out of the mounts.
	if script := renderFileCopies(r.Files); len(script) > 0 {
		fmt.Printf("==> Phase 4: writing %d config file(s)\n", len(r.Files))
		if err := p.lima.Provision(ctx, r.Name, script, p.env(r)); err != nil {
			return fmt.Errorf("phase 4 (files): %w", err)
		}
	}
	// Phase 5 — user scripts, in spec order. Contents are piped in, so a script
	// need not live under a mount. Root runs them with the same env contract the
	// platform scripts get; to call a mise-installed tool, a script drops to the
	// VM user's login shell (`sudo -u "$VM_USER" -H bash -lc '...'`), because
	// root's PATH carries no shims during provisioning.
	for _, sp := range r.Scripts {
		fmt.Printf("==> Phase 5: script — %s\n", sp)
		body, err := os.ReadFile(sp)
		if err != nil {
			return fmt.Errorf("phase 5 (%s): %w", sp, err)
		}
		if err := p.lima.Provision(ctx, r.Name, body, p.env(r)); err != nil {
			return fmt.Errorf("phase 5 (%s): %w", sp, err)
		}
	}
	// Phase 6 — restart, always. It applies group membership (docker),
	// /etc/environment, and anything the guest changed that a live session holds.
	fmt.Printf("==> Phase 6: restarting VM to apply provisioning\n")
	if err := p.lima.Restart(ctx, r.Name); err != nil {
		return fmt.Errorf("phase 6 (restart): %w", err)
	}
	return nil
}
