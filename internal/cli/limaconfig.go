package cli

import (
	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/templates"
	"gopkg.in/yaml.v3"
)

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

	mounts, _ := doc["mounts"].([]any)
	mounts = append(mounts, map[string]any{
		"location":   r.Workspace.HostPath,
		"mountPoint": r.Workspace.GuestPath,
		"writable":   true,
	})
	for _, m := range r.Mounts {
		mounts = append(mounts, map[string]any{
			"location":   m.HostPath,
			"mountPoint": m.GuestPath,
			"writable":   true,
		})
	}
	doc["mounts"] = mounts

	return yaml.Marshal(doc)
}
