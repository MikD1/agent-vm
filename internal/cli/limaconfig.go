package cli

import (
	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/provision"
	"github.com/MikD1/agent-vm/internal/templates"
	"gopkg.in/yaml.v3"
)

// limaMounts renders the full mount list for one VM: the VM directory read-only
// first, then one read/write mount per project. It takes the two fields it needs
// rather than a Resolved, so both create (from Resolved) and the runtime mount
// sync (from a Record) can produce an identical list.
func limaMounts(configDir string, mounts []config.Mount) []any {
	out := []any{map[string]any{
		"location":   configDir,
		"mountPoint": provision.GuestConfigMount,
		"writable":   false,
	}}
	for _, m := range mounts {
		out = append(out, map[string]any{
			"location":   m.HostPath,
			"mountPoint": m.GuestPath,
			"writable":   true,
		})
	}
	return out
}

// buildLimaConfig renders the per-VM Lima YAML from the embedded base template
// plus the resolved config. guestHome sets the user home path.
func buildLimaConfig(r config.Resolved, guestHome string) ([]byte, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(templates.BaseLima, &doc); err != nil {
		return nil, err
	}

	doc["base"] = []any{r.Base.Image}
	doc["cpus"] = r.Resources.CPUs
	doc["memory"] = r.Resources.Memory
	doc["disk"] = r.Resources.Disk
	doc["user"] = map[string]string{"name": r.User, "home": guestHome}
	doc["mounts"] = limaMounts(r.ConfigDir, r.Mounts)

	return yaml.Marshal(doc)
}
