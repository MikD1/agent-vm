package cli

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/modules"
	"github.com/MikD1/agent-vm/internal/registry"
	"github.com/MikD1/agent-vm/internal/vmname"
)

func osUsername() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "user"
}

// externalModuleDir returns the user module dir (~/.config/agent-vm/modules.d) if it exists.
func externalModuleDir() string {
	root, err := registry.DefaultRoot()
	if err != nil {
		return ""
	}
	dir := filepath.Join(root, "modules.d")
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		return dir
	}
	return ""
}

func moduleKnown(name, externalDir string) bool {
	return modules.Exists(name, externalDir)
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
	b := filepath.Base(repo)
	return trimSuffix(b, ".git")
}

func trimSuffix(s, suf string) string {
	if len(s) >= len(suf) && s[len(s)-len(suf):] == suf {
		return s[:len(s)-len(suf)]
	}
	return s
}

// loadSpecForCreate loads the in-repo spec for mount mode (required).
// Clone mode returns an empty spec (flags drive config).
func loadSpecForCreate(f config.Flags, dir string) (config.Spec, bool, string, error) {
	if f.Repo != "" {
		return config.Spec{}, false, "", nil
	}
	specPath := filepath.Join(dir, ".agent-vm.yaml")
	if _, err := os.Stat(specPath); err != nil {
		return config.Spec{}, false, "", errSpecRequired(dir)
	}
	s, err := config.Load(specPath)
	if err != nil {
		return config.Spec{}, false, "", err
	}
	abs, _ := filepath.Abs(dir)
	return s, true, abs, nil
}

func errSpecRequired(dir string) error {
	return fmt.Errorf(".agent-vm.yaml not found in %s (run: avm init)", dir)
}
