package cli

import (
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/registry"
)

func TestFormatList(t *testing.T) {
	withMounts := registry.Record{Name: "alpha", Mounts: []config.Mount{{HostPath: "/h/x", GuestPath: "/g/x"}}}
	entries := []registry.Entry{
		{Name: "alpha", Status: registry.StatusManaged, State: "running", Record: &withMounts},
		{Name: "beta", Status: registry.StatusOrphaned, State: "-"},
		{Name: "gamma", Status: registry.StatusUnmanaged, State: "stopped"},
	}
	out := formatList(entries)
	for _, want := range []string{"STATE", "MOUNTS", "alpha", "managed", "running", "+1", "beta", "orphaned", "gamma", "unmanaged", "stopped"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}
