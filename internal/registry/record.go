// Package registry stores one VM Record per managed VM under <root>/vms and
// reconciles the registry against Lima's view of existence.
package registry

import (
	"time"

	"github.com/MikD1/agent-vm/internal/config"
)

// Record is the host-local materialization of a Project Spec for one Lima VM.
type Record struct {
	Name      string           `yaml:"name"`
	Source    string           `yaml:"source"` // "cli" | "project"
	CreatedAt time.Time        `yaml:"createdAt"`
	Base      config.Base      `yaml:"base"`
	Modules   []string         `yaml:"modules"`
	Resources config.Resources `yaml:"resources"`
	User      string           `yaml:"user"`
	Workspace config.Workspace `yaml:"workspace"`
}

// FromResolved builds a Record from a Resolved config, stamping createdAt.
func FromResolved(r config.Resolved, createdAt time.Time) Record {
	return Record{
		Name:      r.Name,
		Source:    r.Source,
		CreatedAt: createdAt,
		Base:      r.Base,
		Modules:   r.Modules,
		Resources: r.Resources,
		User:      r.User,
		Workspace: r.Workspace,
	}
}
