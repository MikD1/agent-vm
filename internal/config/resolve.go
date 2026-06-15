package config

import (
	"fmt"
	"path"
	"regexp"
)

var (
	sizeRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?[KMGT](iB|B)?$`)
	modRe  = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// Flags are the create/init command-line overrides. ModulesSet records whether
// --modules was passed (cobra Changed), so an unset flag does not shadow the spec.
type Flags struct {
	Modules    []string
	ModulesSet bool
	CPUs       int
	Memory     string
	Disk       string
	BaseImage  string
	Repo       string
	Ref        string
}

// Env carries facts resolved outside config: the normalized project/VM name, the
// guest user/home (from `limactl info`), and—for mount mode—the host project path.
// SpecPresent records whether a spec file was found (→ source "project").
type Env struct {
	ProjectName string
	GuestUser   string
	GuestHome   string
	HostPath    string
	SpecPresent bool
}

// Resolved is the materialized config: everything needed to build both the Lima
// template and the VM Record.
type Resolved struct {
	Name      string
	Source    string // "cli" | "project"
	Modules   []string
	Resources Resources
	Base      Base
	User      string
	Workspace Workspace
}

// Validate checks a Spec in isolation. known reports whether a module name exists.
func (s Spec) Validate(known func(string) bool) error {
	if s.Modules != nil {
		for _, m := range *s.Modules {
			if !modRe.MatchString(m) {
				return fmt.Errorf("invalid module name %q", m)
			}
			if !known(m) {
				return fmt.Errorf("unknown module %q", m)
			}
		}
	}
	if s.Resources.CPUs < 0 {
		return fmt.Errorf("cpus must be positive, got %d", s.Resources.CPUs)
	}
	if s.Resources.Memory != "" && !sizeRe.MatchString(s.Resources.Memory) {
		return fmt.Errorf("invalid memory %q (want a size like 16GiB)", s.Resources.Memory)
	}
	if s.Resources.Disk != "" && !sizeRe.MatchString(s.Resources.Disk) {
		return fmt.Errorf("invalid disk %q (want a size like 120GiB)", s.Resources.Disk)
	}
	return nil
}

// Resolve applies precedence flags > spec > defaults and materializes the workspace.
func Resolve(flags Flags, spec Spec, env Env) (Resolved, error) {
	r := Resolved{
		Name: env.ProjectName,
		User: env.GuestUser,
	}

	// Modules: flag > spec key present > DefaultModules.
	switch {
	case flags.ModulesSet:
		r.Modules = flags.Modules
	case spec.Modules != nil:
		r.Modules = *spec.Modules
	default:
		r.Modules = append([]string(nil), DefaultModules...)
	}

	// Resources: flag > spec > default.
	r.Resources.CPUs = firstInt(flags.CPUs, spec.Resources.CPUs, DefaultCPUs)
	r.Resources.Memory = firstStr(flags.Memory, spec.Resources.Memory, DefaultMemory)
	r.Resources.Disk = firstStr(flags.Disk, spec.Resources.Disk, DefaultDisk)
	r.Base.Image = firstStr(flags.BaseImage, spec.Base.Image, DefaultImage)

	if env.SpecPresent {
		r.Source = "project"
	} else {
		r.Source = "cli"
	}

	guestPath := path.Join(env.GuestHome, env.ProjectName)
	if flags.Repo != "" {
		ref := flags.Ref
		if ref == "" {
			ref = DefaultRef
		}
		r.Workspace = Workspace{Mode: ModeClone, GuestPath: guestPath, Repo: flags.Repo, Ref: ref}
	} else {
		r.Workspace = Workspace{Mode: ModeMount, GuestPath: guestPath, HostPath: env.HostPath}
	}
	return r, nil
}

func firstInt(vals ...int) int {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}

func firstStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
