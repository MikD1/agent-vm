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

// vmName derives the VM name: the spec's `name` if set, otherwise the basename
// of the VM directory. Either way the result goes through vmname.Normalize.
func vmName(spec config.Spec, dir string) (string, error) {
	raw := spec.Name
	if raw == "" {
		raw = filepath.Base(dir)
	}
	return vmname.Normalize(raw)
}

// loadSpecForCreate loads the VM directory's `agent-vm.yaml`, which is required,
// and returns it together with the host path of that directory.
func loadSpecForCreate(dir string) (config.Spec, string, error) {
	specPath := filepath.Join(dir, "agent-vm.yaml")
	if _, err := os.Stat(specPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config.Spec{}, "", errSpecRequired(dir)
		}
		return config.Spec{}, "", err
	}
	s, err := config.Load(specPath)
	if err != nil {
		return config.Spec{}, "", err
	}
	return s, dir, nil
}

func errSpecRequired(dir string) error {
	return fmt.Errorf("agent-vm.yaml not found in %s (run: avm init)", dir)
}

// checkNotVMDir rejects a mount source that is the VM's own directory, or a
// parent folder containing it. That directory is already mounted read-only at
// /mnt/host/vm; mounting it again read/write would hand the guest write access
// to agent-vm.yaml, the CA bundle, and every credential declared in `files` —
// Lima applies both mounts rather than deduplicating them.
func checkNotVMDir(abs, configDir string) error {
	if configDir == "" {
		return nil
	}
	rel, err := filepath.Rel(filepath.Clean(abs), filepath.Clean(configDir))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	return fmt.Errorf("%s is this VM's own directory (or contains it); it is already mounted read-only at /mnt/host/vm", abs)
}

// resolveMountInputs turns Spec mounts into absolute, existence-checked
// MountInputs. This is where the filesystem is touched, keeping the config
// package pure. Relative paths were already rejected by validation.
func resolveMountInputs(specMounts []config.MountSpec, configDir string) ([]config.MountInput, error) {
	var out []config.MountInput
	for _, m := range specMounts {
		abs, err := expandTilde(m.Path)
		if err != nil {
			return nil, err
		}
		abs = filepath.Clean(abs)
		if err := checkNotVMDir(abs, configDir); err != nil {
			return nil, err
		}
		fi, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("mount source not found: %s", abs)
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("mount source is not a directory: %s", abs)
		}
		out = append(out, config.MountInput{HostPath: abs, Name: m.Name})
	}
	return out, nil
}

// resolveFileInputs turns Spec `files` entries into resolved inputs. A source
// must sit under the VM directory, because the copy runs inside the guest and
// that directory is the only host folder a `files` source can be read from.
func resolveFileInputs(specFiles map[string]config.FileSpec, configDir string) ([]config.FileInput, error) {
	var out []config.FileInput
	for src, f := range specFiles {
		abs, err := expandTilde(src)
		if err != nil {
			return nil, err
		}
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(configDir, abs)
		}
		abs = filepath.Clean(abs)

		fi, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("files source %q: %w", src, err)
		}
		rel, err := filepath.Rel(filepath.Clean(configDir), abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("files source %q is outside the VM directory (%s); the guest cannot see it", abs, configDir)
		}
		if fi.IsDir() && f.Mode != "" {
			return nil, fmt.Errorf("files source %q is a directory; remove `mode` (directories keep their own permissions)", src)
		}
		out = append(out, config.FileInput{
			Rel: filepath.ToSlash(rel), To: f.To, Mode: f.Mode, IsDir: fi.IsDir(),
		})
	}
	return out, nil
}

// expandTilde replaces a leading ~/ with the host home. Paths are otherwise
// returned unchanged; callers Clean and check them.
func expandTilde(p string) (string, error) {
	if !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, p[2:]), nil
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
