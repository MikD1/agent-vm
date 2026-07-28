package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".agent-vm.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadFull(t *testing.T) {
	p := writeTemp(t, "modules: [node, claude]\nresources: {cpus: 8, memory: 16GiB, disk: 200GiB}\nbase: {image: corp-ubuntu}\n")
	s, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Modules == nil || len(*s.Modules) != 2 || (*s.Modules)[0] != "node" {
		t.Errorf("modules = %v", s.Modules)
	}
	if s.Resources.CPUs != 8 || s.Resources.Memory != "16GiB" || s.Resources.Disk != "200GiB" {
		t.Errorf("resources = %+v", s.Resources)
	}
	if s.Base.Image != "corp-ubuntu" {
		t.Errorf("base.image = %q", s.Base.Image)
	}
}

func TestLoadModulesAbsentVsEmpty(t *testing.T) {
	absent, err := Load(writeTemp(t, "resources: {cpus: 2}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if absent.Modules != nil {
		t.Errorf("absent modules should be nil, got %v", *absent.Modules)
	}
	empty, err := Load(writeTemp(t, "modules: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	if empty.Modules == nil || len(*empty.Modules) != 0 {
		t.Errorf("empty modules should be non-nil zero-length, got %v", empty.Modules)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("want error for missing file")
	}
}

func TestLoadMounts(t *testing.T) {
	p := writeTemp(t, "modules: [node]\nmounts:\n  - ../shared-lib\n  - path: ../tools/cli\n    name: cli-tools\n")
	s, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Mounts) != 2 {
		t.Fatalf("want 2 mounts, got %d (%+v)", len(s.Mounts), s.Mounts)
	}
	if s.Mounts[0].Path != "../shared-lib" || s.Mounts[0].Name != "" {
		t.Errorf("string form parsed wrong: %+v", s.Mounts[0])
	}
	if s.Mounts[1].Path != "../tools/cli" || s.Mounts[1].Name != "cli-tools" {
		t.Errorf("object form parsed wrong: %+v", s.Mounts[1])
	}
}

func TestLoadNoMounts(t *testing.T) {
	s, err := Load(writeTemp(t, "modules: [node]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Mounts != nil {
		t.Errorf("absent mounts should be nil, got %+v", s.Mounts)
	}
}
