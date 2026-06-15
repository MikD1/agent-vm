// Package modules provides the embedded provisioning bash scripts and optional
// runtime discovery of user-defined modules in an external directory.
package modules

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed scripts/*.sh
var embedded embed.FS

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Script returns the bash for a module: an external <dir>/<name>.sh overrides the
// embedded copy when externalDir is non-empty and the file exists.
func Script(name, externalDir string) ([]byte, error) {
	if !nameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid module name %q", name)
	}
	if externalDir != "" {
		p := filepath.Join(externalDir, name+".sh")
		if b, err := os.ReadFile(p); err == nil {
			return b, nil
		}
	}
	b, err := embedded.ReadFile("scripts/" + name + ".sh")
	if err != nil {
		return nil, fmt.Errorf("module %q not found", name)
	}
	return b, nil
}

// Exists reports whether a module is available (embedded or external).
func Exists(name, externalDir string) bool {
	_, err := Script(name, externalDir)
	return err == nil
}

// List returns the names of all embedded modules, sorted.
func List() []string {
	entries, _ := embedded.ReadDir("scripts")
	var out []string
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), ".sh"))
	}
	sort.Strings(out)
	return out
}
