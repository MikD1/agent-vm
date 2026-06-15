// Package provision drives the fixed phase sequence (create/start → system →
// base → feature modules → workspace → optional restart) via a lima.Client.
package provision

import (
	"context"
	"fmt"
	"strings"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/modules"
)

// Provisioner runs the phases for one VM.
type Provisioner struct {
	lima        *lima.Client
	externalDir string // user module dir; "" disables external discovery
}

// New builds a Provisioner.
func New(c *lima.Client, externalDir string) *Provisioner {
	return &Provisioner{lima: c, externalDir: externalDir}
}

func (p *Provisioner) env(r config.Resolved) map[string]string {
	return map[string]string{
		"VM_USER":      r.User,
		"VM_PROJECT":   r.Name,
		"VM_WORKSPACE": r.Workspace.GuestPath,
		"VM_SECRETS":   "/mnt/host/agent-vm",
	}
}

func (p *Provisioner) provisionModule(ctx context.Context, r config.Resolved, name string) error {
	script, err := modules.Script(name, p.externalDir)
	if err != nil {
		return err
	}
	return p.lima.Provision(ctx, r.Name, script, p.env(r))
}

// Run executes the full sequence. The caller (cli create) handles VM rollback on
// any returned error.
func (p *Provisioner) Run(ctx context.Context, r config.Resolved, limaConfigPath string) error {
	// Phase 0 — create + start.
	if err := p.lima.Create(ctx, r.Name, limaConfigPath); err != nil {
		return err
	}
	if err := p.lima.Start(ctx, r.Name); err != nil {
		return err
	}
	// Phase 1 — system layer.
	if err := p.provisionModule(ctx, r, "system"); err != nil {
		return fmt.Errorf("phase 1 (system): %w", err)
	}
	// Phase 2 — base module.
	if err := p.provisionModule(ctx, r, "base"); err != nil {
		return fmt.Errorf("phase 2 (base): %w", err)
	}
	// Phase 3 — feature modules in spec order.
	needsRestart := false
	for _, m := range r.Modules {
		if err := p.provisionModule(ctx, r, m); err != nil {
			return fmt.Errorf("phase 3 (%s): %w", m, err)
		}
		if m == "docker" {
			needsRestart = true
		}
	}
	// Phase 4 — workspace (clone only; mount is already present via virtiofs).
	if r.Workspace.Mode == config.ModeClone {
		if err := p.cloneWorkspace(ctx, r); err != nil {
			return fmt.Errorf("phase 4 (clone): %w", err)
		}
	}
	// Post — restart only to apply docker group membership.
	if needsRestart {
		if err := p.lima.Restart(ctx, r.Name); err != nil {
			return err
		}
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
