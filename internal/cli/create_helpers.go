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

// resolveFileInputs turns Spec `files` entries into resolved inputs. Sources are
// relative to the spec dir, or absolute/~-prefixed on the host; this is where the
// filesystem is touched, keeping the config package pure.
//
// A source must sit under one of the two directories the guest can see — the
// project dir (VM_WORKSPACE) or the host store (VM_SECRETS) — because the copy
// runs inside the guest.
func resolveFileInputs(specFiles map[string]config.FileSpec, specDir, storeRoot string) ([]config.FileInput, error) {
	var out []config.FileInput
	for src, f := range specFiles {
		abs := src
		if strings.HasPrefix(abs, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			abs = filepath.Join(home, abs[2:])
		}
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(specDir, abs)
		}
		abs = filepath.Clean(abs)

		fi, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("files source %q: %w", src, err)
		}

		root, rel, err := classifyFileRoot(abs, specDir, storeRoot)
		if err != nil {
			return nil, err
		}
		if fi.IsDir() && f.Mode != "" {
			return nil, fmt.Errorf("files source %q is a directory; remove `mode` (directories keep their own permissions)", src)
		}
		out = append(out, config.FileInput{
			Root: root, Rel: rel, To: f.To, Mode: f.Mode, IsDir: fi.IsDir(),
		})
	}
	return out, nil
}

// resolveScriptInputs turns Spec `scripts` entries (relative to the spec dir)
// into absolute host paths, checking each exists before the VM is created. Order
// is preserved: scripts run in the order the Spec lists them.
func resolveScriptInputs(specScripts []string, specDir string) ([]string, error) {
	out := make([]string, 0, len(specScripts))
	for _, s := range specScripts {
		abs := s
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(specDir, abs)
		}
		abs = filepath.Clean(abs)
		if _, err := os.Stat(abs); err != nil {
			return nil, fmt.Errorf("scripts entry %q: %w", s, err)
		}
		out = append(out, abs)
	}
	return out, nil
}

// classifyFileRoot decides which mounted root a source belongs to and its path
// relative to that root.
func classifyFileRoot(abs, specDir, storeRoot string) (config.FileRoot, string, error) {
	for _, c := range []struct {
		root config.FileRoot
		base string
	}{
		{config.RootWorkspace, filepath.Clean(specDir)},
		{config.RootSecrets, filepath.Clean(storeRoot)},
	} {
		rel, err := filepath.Rel(c.base, abs)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return c.root, filepath.ToSlash(rel), nil
		}
	}
	return "", "", fmt.Errorf("files source %q is outside both the project directory (%s) and the host store (%s); the guest cannot see it", abs, specDir, storeRoot)
}
