package config

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

var (
	sizeRe      = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?[KMGT](iB|B)?$`)
	moduleRefRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]+(:@?[a-zA-Z0-9_./-]+)?$`)
	moduleVerRe = regexp.MustCompile(`^[^\s"']+$`)
	mountNameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	fileModeRe  = regexp.MustCompile(`^0[0-7]{3}$`)
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
}

// Mount is a resolved host→guest folder mapping: one project this VM works on,
// mounted read/write at its guest path.
type Mount struct {
	HostPath  string `yaml:"hostPath"`
	GuestPath string `yaml:"guestPath"`
}

// MountInput is an already-resolved (absolute) host folder fed into Resolve via
// Env. The CLI layer expands ~/ and checks existence, keeping config free of
// filesystem access.
type MountInput struct {
	HostPath string
	Name     string
}

// FileInput is one `files` entry after the CLI has checked it exists. Rel is the
// path relative to the VM directory; To is still in ~/ form, and Resolve expands it.
type FileInput struct {
	Rel   string
	To    string
	Mode  string
	IsDir bool
}

// FileCopy is a resolved `files` entry: an absolute guest destination plus the
// path relative to the VM directory that the guest reconstructs its source from.
type FileCopy struct {
	Rel   string `yaml:"rel"`
	To    string `yaml:"to"`
	Mode  string `yaml:"mode,omitempty"`
	IsDir bool   `yaml:"isDir,omitempty"`
}

// Env carries facts resolved outside config: the normalized VM name, the guest
// user/home (from `limactl info`), and the host path of the VM directory.
type Env struct {
	ProjectName string
	GuestUser   string
	GuestHome   string
	ConfigDir   string
	Mounts      []MountInput
	Files       []FileInput
	Scripts     []string
}

// Resolved is the materialized config: everything needed to build both the Lima
// template and the VM Record.
type Resolved struct {
	Name      string
	Modules   []ModuleSpec
	Resources Resources
	Base      Base
	User      string
	ConfigDir string
	Home      string
	Mounts    []Mount
	Files     []FileCopy
	Scripts   []string
}

// Validate checks a Spec in isolation. Module names are mise tool references and
// are not checked against a catalog: avm does not know the mise registry offline,
// so an unknown name surfaces when mise runs.
func (s Spec) Validate() error {
	if s.Name != "" {
		// ValidateMountName's message already names the offending value and the
		// rule, so it is returned as-is rather than restated.
		if err := ValidateMountName(s.Name); err != nil {
			return err
		}
	}
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
		if !strings.HasPrefix(m.Path, "/") && !strings.HasPrefix(m.Path, "~/") {
			return fmt.Errorf("mount path %q must be absolute or start with ~/", m.Path)
		}
		if hasDotDot(m.Path) {
			return fmt.Errorf("mount path %q must not contain ..", m.Path)
		}
		if m.Name != "" {
			if err := ValidateMountName(m.Name); err != nil {
				return err
			}
		}
	}
	dest := map[string]string{}
	for src, f := range s.Files {
		if src == "" {
			return fmt.Errorf("files entry has an empty source path")
		}
		if f.To == "" {
			return fmt.Errorf("files entry %q has an empty destination", src)
		}
		if !strings.HasPrefix(f.To, "/") && !strings.HasPrefix(f.To, "~/") {
			return fmt.Errorf("files destination %q must be absolute or start with ~/", f.To)
		}
		if hasDotDot(f.To) {
			return fmt.Errorf("files destination %q must not contain ..", f.To)
		}
		if f.Mode != "" && !fileModeRe.MatchString(f.Mode) {
			return fmt.Errorf("invalid mode %q for %q (want four octal digits, e.g. 0600)", f.Mode, src)
		}
		if prev, clash := dest[f.To]; clash {
			return fmt.Errorf("files conflict: %q and %q both write %s", src, prev, f.To)
		}
		dest[f.To] = src
	}
	for _, s2 := range s.Scripts {
		if s2 == "" {
			return fmt.Errorf("scripts entry is empty")
		}
		if hasDotDot(s2) {
			return fmt.Errorf("scripts entry %q must not contain ..", s2)
		}
	}
	return nil
}

// ValidateMountName checks a guest directory name: a single path segment. It is
// exported because a mount can also be named at runtime, outside a Spec, and it
// doubles as the check on a Spec's own `name` — hence the context-free message.
func ValidateMountName(name string) error {
	if name == "." || name == ".." || !mountNameRe.MatchString(name) {
		return fmt.Errorf("invalid name %q (use a single path segment)", name)
	}
	return nil
}

// Resolve applies precedence flags > spec > defaults and materializes the mounts.
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

	r.ConfigDir = env.ConfigDir
	r.Home = env.GuestHome
	mounts, err := resolveMounts(env.Mounts, env.GuestHome)
	if err != nil {
		return Resolved{}, err
	}
	r.Mounts = mounts
	r.Files = resolveFiles(env.Files, env.GuestHome)
	// Scripts run in spec order, so this list is copied, never sorted.
	r.Scripts = append([]string(nil), env.Scripts...)
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

// resolveMounts computes each mount's guest path (~/<name>), dedupes identical
// host paths, and rejects guest-path collisions between two different folders.
func resolveMounts(inputs []MountInput, guestHome string) ([]Mount, error) {
	var out []Mount
	seenHost := map[string]bool{}
	owner := map[string]string{}
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
			return nil, fmt.Errorf("mount conflict: %q and %q both map to %s; pass a different name", in.HostPath, prev, guest)
		}
		owner[guest] = in.HostPath
		out = append(out, Mount{HostPath: in.HostPath, GuestPath: guest})
	}
	return out, nil
}

// hasDotDot reports whether any path segment is "..".
func hasDotDot(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// resolveFiles expands ~/ destinations against the guest home, applies the
// default mode, and sorts by destination so the generated script and the Record
// are byte-stable.
func resolveFiles(inputs []FileInput, guestHome string) []FileCopy {
	out := make([]FileCopy, 0, len(inputs))
	for _, in := range inputs {
		to := in.To
		if strings.HasPrefix(to, "~/") {
			to = path.Join(guestHome, to[2:])
		}
		mode := in.Mode
		if mode == "" && !in.IsDir {
			mode = DefaultFileMode
		}
		out = append(out, FileCopy{Rel: in.Rel, To: to, Mode: mode, IsDir: in.IsDir})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].To < out[j].To })
	return out
}
