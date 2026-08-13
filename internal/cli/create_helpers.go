package cli

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/vmname"
)

func osUsername() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "user"
}

// projectName derives the VM name: for clone mode, from the repo basename;
// otherwise from the directory basename.
func projectName(f config.Flags, dir string) (string, error) {
	if f.Repo != "" {
		return vmname.Normalize(repoBasename(f.Repo))
	}
	return vmname.Normalize(filepath.Base(dir))
}

func repoBasename(repo string) string {
	return strings.TrimSuffix(filepath.Base(repo), ".git")
}

// loadSpecForCreate loads the in-repo spec for mount mode (required).
// Clone mode returns an empty spec (flags drive config).
func loadSpecForCreate(f config.Flags, dir string) (config.Spec, bool, string, error) {
	if f.Repo != "" {
		return config.Spec{}, false, "", nil
	}
	specPath := filepath.Join(dir, ".agent-vm.yaml")
	if _, err := os.Stat(specPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config.Spec{}, false, "", errSpecRequired(dir)
		}
		return config.Spec{}, false, "", err
	}
	s, err := config.Load(specPath)
	if err != nil {
		return config.Spec{}, false, "", err
	}
	return s, true, dir, nil
}

func errSpecRequired(dir string) error {
	return fmt.Errorf(".agent-vm.yaml not found in %s (run: avm init)", dir)
}

// resolveMountInputs turns Spec mounts (paths relative to specDir) and --mount
// flags (relative to cwd) into absolute, existence-checked MountInputs. This is
// where the filesystem is touched, keeping the config package pure.
func resolveMountInputs(specMounts []config.MountSpec, flagMounts []string, specDir, cwd string) ([]config.MountInput, error) {
	var out []config.MountInput
	add := func(rawPath, base, name string) error {
		abs := rawPath
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(base, abs)
		}
		abs = filepath.Clean(abs)
		fi, err := os.Stat(abs)
		if err != nil {
			return fmt.Errorf("mount source not found: %s", abs)
		}
		if !fi.IsDir() {
			return fmt.Errorf("mount source is not a directory: %s", abs)
		}
		out = append(out, config.MountInput{HostPath: abs, Name: name})
		return nil
	}
	for _, m := range specMounts {
		if err := add(m.Path, specDir, m.Name); err != nil {
			return nil, err
		}
	}
	for _, p := range flagMounts {
		if err := add(p, cwd, ""); err != nil {
			return nil, err
		}
	}
	return out, nil
}
