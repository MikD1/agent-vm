package cli

import (
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/registry"
)

func TestFormatList(t *testing.T) {
	entries := []registry.Entry{
		{Name: "alpha", Status: registry.StatusManaged},
		{Name: "beta", Status: registry.StatusOrphaned},
		{Name: "gamma", Status: registry.StatusUnmanaged},
	}
	out := formatList(entries)
	for _, want := range []string{"alpha", "managed", "beta", "orphaned", "gamma", "unmanaged"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}
