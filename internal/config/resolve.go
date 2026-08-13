package config

import (
	"fmt"
	"path"
	"regexp"
)

var (
	sizeRe       = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?[KMGT](iB|B)?$`)
	moduleRefRe  = regexp.MustCompile(`^[a-zA-Z0-9_.-]+(:@?[a-zA-Z0-9_./-]+)?$`)
	moduleVerRe  = regexp.MustCompile(`^[^\s"']+$`)
	mountNameRe  = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
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

// Mount is a resolved additional mount: absolute host + guest paths (materialization).
type Mount struct {
	HostPath  string `yaml:"hostPath"`
	GuestPath string `yaml:"guestPath"`
}

// MountInput is an already-resolved (absolute) additional host folder fed into
// Resolve via Env. The CLI layer does relative-path resolution and existence
// checks, keeping config free of filesystem access.
type MountInput struct {
	HostPath string
	Name     string
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
	Mounts      []MountInput
}

// Resolved is the materialized config: everything needed to build both the Lima
// template and the VM Record.
type Resolved struct {
	Name      string
	Source    string // "cli" | "project"
	Modules   []ModuleSpec
	Resources Resources
	Base      Base
	User      string
	Workspace Workspace
	Mounts    []Mount
}

// Validate checks a Spec in isolation. Module names are mise tool references and
// are not checked against a catalog: avm does not know the mise registry offline,
// so an unknown name surfaces when mise runs.
func (s Spec) Validate() error {
	if s.Modules != nil {
		seen := map[string]bool{}
		for _, m := range *s.Modules {
			if !moduleRefRe.MatchString(m.Name) {
				return fmt.Errorf("invalid module name %q", m.Name)
			}
			if m.Version != "" && !moduleVerRe.MatchString(m.Version) {
				return fmt.Errorf("invalid version %q for module %q", m.Version, m.Name)
			}
			if seen[m.Name] {
				return fmt.Errorf("module %q listed twice", m.Name)
			}
			seen[m.Name] = true
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
	for _, m := range s.Mounts {
		if m.Path == "" {
			return fmt.Errorf("mount entry has empty path")
		}
		if m.Name != "" {
			if m.Name == "." || m.Name == ".." || !mountNameRe.MatchString(m.Name) {
				return fmt.Errorf("invalid mount name %q (use a single path segment)", m.Name)
			}
		}
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
		r.Modules = make([]ModuleSpec, 0, len(flags.Modules))
		for _, raw := range flags.Modules {
			r.Modules = append(r.Modules, ParseModuleRef(raw))
		}
	case spec.Modules != nil:
		r.Modules = append([]ModuleSpec(nil), *spec.Modules...)
	default:
		r.Modules = append([]ModuleSpec(nil), DefaultModules...)
	}
	// A module named without a version installs the latest release.
	for i := range r.Modules {
		if r.Modules[i].Version == "" {
			r.Modules[i].Version = DefaultToolVersion
		}
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
	mounts, err := resolveMounts(env.Mounts, env.GuestHome, r.Workspace.GuestPath)
	if err != nil {
		return Resolved{}, err
	}
	r.Mounts = mounts
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

// resolveMounts computes each additional mount's guest path (~/<name>), dedupes
// identical host paths, and rejects guest-path collisions — including a clash
// with the primary workspace.
func resolveMounts(inputs []MountInput, guestHome, primaryGuest string) ([]Mount, error) {
	var out []Mount
	seenHost := map[string]bool{}
	owner := map[string]string{primaryGuest: "the primary workspace"}
	for _, in := range inputs {
		if seenHost[in.HostPath] {
			continue
		}
		seenHost[in.HostPath] = true
		name := in.Name
		if name == "" {
			name = path.Base(in.HostPath)
		}
		guest := path.Join(guestHome, name)
		if prev, clash := owner[guest]; clash {
			return nil, fmt.Errorf("mount conflict: %q and %s both map to %s; set an explicit name on one of them", in.HostPath, prev, guest)
		}
		owner[guest] = fmt.Sprintf("%q", in.HostPath)
		out = append(out, Mount{HostPath: in.HostPath, GuestPath: guest})
	}
	return out, nil
}
