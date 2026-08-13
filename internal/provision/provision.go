// Package provision drives the fixed phase sequence (create/start → system →
// platform → tools → workspace → restart) via a lima.Client.
package provision

import (
	"context"
	"fmt"
	"strings"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/lima"
)

// Provisioner runs the phases for one VM.
type Provisioner struct {
	lima    *lima.Client
	verbose bool // mirrors --verbose onto the tools the guest runs
}

// New builds a Provisioner.
func New(c *lima.Client, verbose bool) *Provisioner {
	return &Provisioner{lima: c, verbose: verbose}
}

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
	// Phase 4 — workspace (clone only; mount is already present via virtiofs).
	if r.Workspace.Mode == config.ModeClone {
		fmt.Printf("==> Phase 4: cloning workspace\n")
		if err := p.cloneWorkspace(ctx, r); err != nil {
			return fmt.Errorf("phase 4 (clone): %w", err)
		}
	}
	// Phase 7 — restart, always. It applies group membership (docker),
	// /etc/environment, and anything the guest changed that a live session holds.
	fmt.Printf("==> Restarting VM to apply provisioning\n")
	if err := p.lima.Restart(ctx, r.Name); err != nil {
		return err
	}
	return nil
}

// cloneWorkspace runs `git clone` inside the guest as the VM user via the
// forwarded SSH agent.
func (p *Provisioner) cloneWorkspace(ctx context.Context, r config.Resolved) error {
	script := fmt.Sprintf("sudo -u %s -H git clone --branch %s %s %s",
		shellQuote(r.User), shellQuote(r.Workspace.Ref),
		shellQuote(r.Workspace.Repo), shellQuote(r.Workspace.GuestPath))
	return p.lima.Provision(ctx, r.Name, []byte(script), p.env(r))
}

// shellQuote wraps s in single quotes and escapes any embedded single quotes,
// producing a bash-safe argument regardless of the string's content.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
