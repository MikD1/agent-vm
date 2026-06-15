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

// externalModuleDir returns the user module dir (<root>/modules.d) if it exists.
func externalModuleDir(root string) string {
	dir := filepath.Join(root, "modules.d")
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		return dir
	}
	return ""
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
