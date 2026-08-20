// Package config defines the VM Spec, its validation, and the resolution funnel
// (flags > VM Spec > built-in defaults) that produces a Resolved config feeding
// both the Lima template and the VM Record.
package config

import (
	"fmt"
	"os"
	"strings"

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

// MountSpec is one project folder in the VM Spec. It accepts either a bare
// string (the path) or a mapping {path, name}. Paths are absolute or ~/-prefixed
// on the host; name overrides the guest directory name, which defaults to the
// path's basename.
type MountSpec struct {
	Path string `yaml:"path"`
	Name string `yaml:"name,omitempty"`
}

// UnmarshalYAML lets a mount be written as a scalar path or a {path, name} map.
func (m *MountSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&m.Path)
	}
	type raw MountSpec // avoid recursing into this UnmarshalYAML
	var r raw
	if err := node.Decode(&r); err != nil {
		return err
	}
	*m = MountSpec(r)
	return nil
}

// ModuleSpec is one tool in the VM Spec: a mise tool reference plus an
// optional version. It accepts either a bare scalar (the name) or a single-key
// mapping (name → version) — the same scalar-or-map shape MountSpec uses.
type ModuleSpec struct {
	Name    string
	Version string
}

// UnmarshalYAML reads both accepted forms. Scalar values are taken from
// node.Value rather than decoded, so an unquoted YAML number (`node: 22`) is
// read as the string "22" instead of failing as an int.
func (m *ModuleSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		m.Name = node.Value
		return nil
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 2 {
		return fmt.Errorf("module entry must be a name or a single `name: version` pair")
	}
	key, val := node.Content[0], node.Content[1]
	if key.Kind != yaml.ScalarNode || val.Kind != yaml.ScalarNode {
		return fmt.Errorf("module entry must be a single `name: version` pair")
	}
	m.Name, m.Version = key.Value, val.Value
	return nil
}

// MarshalYAML writes the compact form, so a VM Record round-trips as
// `- node: lts` rather than as a two-key struct.
func (m ModuleSpec) MarshalYAML() (any, error) {
	if m.Version == "" {
		return m.Name, nil
	}
	return map[string]string{m.Name: m.Version}, nil
}

// ParseModuleRef splits a --modules entry written in mise syntax (`node@lts`).
// The split uses the last `@` only when it comes after the last `/`, so a scoped
// npm package (`npm:@openai/codex`) is not mistaken for a version.
func ParseModuleRef(s string) ModuleSpec {
	if at := strings.LastIndex(s, "@"); at > 0 && at > strings.LastIndex(s, "/") {
		return ModuleSpec{Name: s[:at], Version: s[at+1:]}
	}
	return ModuleSpec{Name: s}
}

// FileSpec is the destination of one `files` entry. It accepts either a bare
// scalar (the guest path) or a mapping {to, mode}.
type FileSpec struct {
	To   string `yaml:"to"`
	Mode string `yaml:"mode,omitempty"`
}

// UnmarshalYAML lets a destination be written as a scalar path or a {to, mode} map.
func (f *FileSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		f.To = node.Value
		return nil
	}
	type raw FileSpec // avoid recursing into this UnmarshalYAML
	var r raw
	if err := node.Decode(&r); err != nil {
		return err
	}
	*f = FileSpec(r)
	return nil
}

// Spec is the human-authored VM Spec (agent-vm.yaml): one domain of work — its
// tools, its resources, and the projects mounted into it. Modules is a pointer
// so an absent key (nil → defaults may apply) is distinguishable from an
// explicit empty list (base only).
type Spec struct {
	Name      string              `yaml:"name,omitempty"`
	Modules   *[]ModuleSpec       `yaml:"modules,omitempty"`
	Resources Resources           `yaml:"resources,omitempty"`
	Base      Base                `yaml:"base,omitempty"`
	Mounts    []MountSpec         `yaml:"mounts,omitempty"`
	Files     map[string]FileSpec `yaml:"files,omitempty"`
	Scripts   []string            `yaml:"scripts,omitempty"`
}

// Load parses an agent-vm.yaml file into a Spec.
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
