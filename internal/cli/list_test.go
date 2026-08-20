package cli

import (
	"context"
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

func TestFormatDetail(t *testing.T) {
	rec := registry.Record{
		Name: "work", User: "me",
		Base:      config.Base{Image: "template:_images/ubuntu"},
		Resources: config.Resources{CPUs: 8, Memory: "16GiB", Disk: "200GiB"},
		ConfigDir: "/Users/me/vms/work",
		Home:      "/home/me.linux",
		Mounts: []config.Mount{
			{HostPath: "/Users/me/projects/api", GuestPath: "/home/me.linux/api"},
		},
		InstalledTools: []config.ModuleSpec{{Name: "node", Version: "22.9.0"}},
	}
	out := formatDetail(registry.Entry{
		Name: "work", Status: registry.StatusManaged, State: "running", Record: &rec,
	})
	for _, want := range []string{
		"work", "managed", "running",
		"template:_images/ubuntu", "8", "16GiB", "200GiB",
		"/Users/me/vms/work",
		"MOUNTS", "/Users/me/projects/api", "/home/me.linux/api",
		"TOOLS", "node", "22.9.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("detail output missing %q:\n%s", want, out)
		}
	}
}

func TestFormatDetailEmptySections(t *testing.T) {
	rec := registry.Record{Name: "work", ConfigDir: "/Users/me/vms/work"}
	out := formatDetail(registry.Entry{
		Name: "work", Status: registry.StatusManaged, State: "stopped", Record: &rec,
	})
	if strings.Count(out, "(none)") != 2 {
		t.Errorf("a VM with no mounts and no tools must show (none) twice:\n%s", out)
	}
}

func TestRunListOneVM(t *testing.T) {
	store := registry.NewStore(t.TempDir())
	_ = store.Write(registry.Record{Name: "work", ConfigDir: "/Users/me/vms/work"})
	c := stubNames([]string{"work"})

	out, err := runList(context.Background(), c, store, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "MOUNTS") {
		t.Errorf("named list must render the detail block:\n%s", out)
	}
	if strings.Contains(out, "NAME       STATUS") {
		t.Errorf("named list must not render the table header:\n%s", out)
	}
	if _, err := runList(context.Background(), c, store, "ghost"); err == nil {
		t.Error("want an error for an unknown VM name")
	}
}

func TestListCmdAcceptsOptionalName(t *testing.T) {
	cmd := newListCmd()
	if err := cmd.Args(cmd, []string{}); err != nil {
		t.Errorf("avm list with no args must be valid: %v", err)
	}
	if err := cmd.Args(cmd, []string{"work"}); err != nil {
		t.Errorf("avm list <name> must be valid: %v", err)
	}
	if err := cmd.Args(cmd, []string{"a", "b"}); err == nil {
		t.Error("avm list takes at most one argument")
	}
}
