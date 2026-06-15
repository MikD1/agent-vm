// Package config defines the Project Spec, its validation, and the resolution
// funnel (flags > in-repo spec > built-in defaults) that produces a Resolved
// config feeding both the Lima template and the VM Record.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Resources are per-VM resource overrides; zero values mean "use the default".
type Resources struct {
	CPUs   int    `yaml:"cpus,omitempty"`
	Memory string `yaml:"memory,omitempty"`
	Disk   string `yaml:"disk,omitempty"`
}

// Base selects the Lima base image.
type Base struct {
	Image string `yaml:"image,omitempty"`
}

// Workspace is the RESOLVED workspace (mode + paths). It lives in config so the
// registry can reuse it; the Project Spec itself carries no workspace.
type Workspace struct {
	Mode      string `yaml:"mode"`               // "mount" | "clone"
	GuestPath string `yaml:"guestPath"`          // absolute path to code in the guest
	HostPath  string `yaml:"hostPath,omitempty"` // mount mode
	Repo      string `yaml:"repo,omitempty"`     // clone mode
	Ref       string `yaml:"ref,omitempty"`      // clone mode
}

// Spec is the human-authored Project Spec (.agent-vm.yaml). Modules is a pointer
// so an absent key (nil → defaults may apply) is distinguishable from an explicit
// empty list (base only).
type Spec struct {
	Modules   *[]string `yaml:"modules,omitempty"`
	Resources Resources `yaml:"resources,omitempty"`
	Base      Base      `yaml:"base,omitempty"`
}

// Load parses a .agent-vm.yaml file into a Spec.
func Load(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, fmt.Errorf("read spec: %w", err)
	}
	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return Spec{}, fmt.Errorf("parse spec %s: %w", path, err)
	}
	return s, nil
}
