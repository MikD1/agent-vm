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

	doc["base"] = []map[string]string{{"location": r.Base.Image}}
	doc["cpus"] = r.Resources.CPUs
	doc["memory"] = r.Resources.Memory
	doc["disk"] = r.Resources.Disk
	doc["user"] = map[string]string{"name": r.User, "home": guestHome}

	mounts, _ := doc["mounts"].([]any)
	if r.Workspace.Mode == config.ModeMount {
		mounts = append(mounts, map[string]any{
			"location":   r.Workspace.HostPath,
			"mountPoint": r.Workspace.GuestPath,
			"writable":   true,
		})
	}
	doc["mounts"] = mounts

	ssh, _ := doc["ssh"].(map[string]any)
	if ssh == nil {
		ssh = map[string]any{}
	}
	ssh["forwardAgent"] = r.Workspace.Mode == config.ModeClone
	doc["ssh"] = ssh

	return yaml.Marshal(doc)
}
