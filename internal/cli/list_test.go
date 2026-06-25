package cli

import (
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/registry"
)

func TestFormatList(t *testing.T) {
	entries := []registry.Entry{
		{Name: "alpha", Status: registry.StatusManaged, State: "running"},
		{Name: "beta", Status: registry.StatusOrphaned, State: "-"},
		{Name: "gamma", Status: registry.StatusUnmanaged, State: "stopped"},
	}
	out := formatList(entries)
	for _, want := range []string{"STATE", "alpha", "managed", "running", "beta", "orphaned", "-", "gamma", "unmanaged", "stopped"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}
