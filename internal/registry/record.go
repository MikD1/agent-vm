// Package registry stores one VM Record per managed VM under <root>/vms and
// reconciles the registry against Lima's view of existence.
package registry

import (
	"time"

	"github.com/MikD1/agent-vm/internal/config"
)

// Record is the host-local materialization of a VM Spec for one Lima VM.
type Record struct {
	Name      string              `yaml:"name"`
	CreatedAt time.Time           `yaml:"createdAt"`
	Base      config.Base         `yaml:"base"`
	Modules   []config.ModuleSpec `yaml:"modules"`
	// InstalledTools is what mise actually resolved, filled in after a successful
	// provision. Modules is the intent; this is the materialization.
	InstalledTools []config.ModuleSpec `yaml:"installedTools,omitempty"`
	Resources      config.Resources    `yaml:"resources"`
	User           string              `yaml:"user"`
	// ConfigDir is the absolute host path of the VM directory. Without it `avm
	// mount`, run from a project folder, could not find the config to update.
	ConfigDir string `yaml:"configDir"`
	// Home is the guest home resolved at create time. `avm mount` needs it to
	// build a new mount's guest path, and `avm shell` to pick its workdir:
	// `limactl info` reports the home of the template, not of this VM, so
	// reading it later would depend on the Lima version installed at that
	// moment. The Record is a snapshot of creation facts, and home is one of
	// them.
	Home string `yaml:"home"`
	// Mounts carries no omitempty: an empty mount list is a valid and meaningful
	// state, and it should be visible in the file.
	Mounts  []config.Mount    `yaml:"mounts"`
	Files   []config.FileCopy `yaml:"files,omitempty"`
	Scripts []string          `yaml:"scripts,omitempty"`
}

// FromResolved builds a Record from a Resolved config, stamping createdAt.
func FromResolved(r config.Resolved, createdAt time.Time) Record {
	return Record{
		Name:      r.Name,
		CreatedAt: createdAt,
		Base:      r.Base,
		Modules:   r.Modules,
		Resources: r.Resources,
		User:      r.User,
		ConfigDir: r.ConfigDir,
		Home:      r.Home,
		Mounts:    r.Mounts,
		Files:     r.Files,
		Scripts:   r.Scripts,
	}
}
