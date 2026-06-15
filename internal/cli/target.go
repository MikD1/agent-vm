package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/MikD1/agent-vm/internal/vmname"
	"gopkg.in/yaml.v3"
)

var nonUser = regexp.MustCompile(`[^a-z0-9_-]`)

// deriveGuestUser turns the host username into a valid Linux username (Lima rules).
func deriveGuestUser(hostUser string) string {
	u := nonUser.ReplaceAllString(strings.ToLower(hostUser), "_")
	if u == "" || !regexp.MustCompile(`^[a-z_]`).MatchString(u) {
		u = "_" + u
	}
	if len(u) > 32 {
		u = u[:32]
	}
	return u
}

// guestHome derives the guest home for vmUser from `limactl info` JSON, replacing
// the default template user with vmUser. Falls back to /home/<user>.linux.
func guestHome(infoJSON []byte, vmUser string) (string, error) {
	var info struct {
		DefaultTemplate struct {
			User struct {
				Name string `yaml:"name"`
				Home string `yaml:"home"`
			} `yaml:"user"`
		} `yaml:"defaultTemplate"`
	}
	// limactl info is JSON, which is valid YAML — reuse the yaml decoder.
	if err := yaml.Unmarshal(infoJSON, &info); err != nil {
		return "", fmt.Errorf("parse limactl info: %w", err)
	}
	name, home := info.DefaultTemplate.User.Name, info.DefaultTemplate.User.Home
	if name != "" && home != "" {
		return strings.Replace(home, name, vmUser, 1), nil
	}
	return "/home/" + vmUser + ".linux", nil
}

// resolveTargetName implements the uniform target rule: explicit arg > cwd spec >
// error. dir is the working directory (cwd) to inspect when name is empty.
func resolveTargetName(name, dir string) (string, error) {
	if name != "" {
		return vmname.Normalize(name)
	}
	spec := filepath.Join(dir, ".agent-vm.yaml")
	if _, err := os.Stat(spec); err != nil {
		return "", fmt.Errorf("no .agent-vm.yaml in %s; pass a VM name or cd into a project", dir)
	}
	return vmname.Normalize(filepath.Base(dir))
}
