package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "agent-vm.yaml")
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
	if s.Modules == nil || len(*s.Modules) != 2 || (*s.Modules)[0].Name != "node" {
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

func TestLoadName(t *testing.T) {
	s, err := Load(writeTemp(t, "name: work\nmodules: [node]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "work" {
		t.Errorf("name = %q, want work", s.Name)
	}
	absent, err := Load(writeTemp(t, "modules: [node]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if absent.Name != "" {
		t.Errorf("absent name = %q, want empty", absent.Name)
	}
}

func TestLoadMounts(t *testing.T) {
	p := writeTemp(t, "modules: [node]\nmounts:\n  - ~/projects/api\n  - path: /Users/me/tools/cli\n    name: cli-tools\n")
	s, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Mounts) != 2 {
		t.Fatalf("want 2 mounts, got %d (%+v)", len(s.Mounts), s.Mounts)
	}
	if s.Mounts[0].Path != "~/projects/api" || s.Mounts[0].Name != "" {
		t.Errorf("string form parsed wrong: %+v", s.Mounts[0])
	}
	if s.Mounts[1].Path != "/Users/me/tools/cli" || s.Mounts[1].Name != "cli-tools" {
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

func TestModuleSpecForms(t *testing.T) {
	const y = `
modules:
  - node
  - go: "1.24"
  - python: 3.12
  - "npm:@openai/codex"
`
	var s Spec
	if err := yaml.Unmarshal([]byte(y), &s); err != nil {
		t.Fatal(err)
	}
	if s.Modules == nil {
		t.Fatal("modules is nil")
	}
	want := []ModuleSpec{
		{Name: "node"},
		{Name: "go", Version: "1.24"},
		{Name: "python", Version: "3.12"}, // unquoted YAML number → string
		{Name: "npm:@openai/codex"},
	}
	got := *s.Modules
	if len(got) != len(want) {
		t.Fatalf("got %d modules, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("modules[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestModuleSpecRoundTrip(t *testing.T) {
	in := []ModuleSpec{{Name: "node", Version: "lts"}, {Name: "claude"}}
	out, err := yaml.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "- node: lts\n- claude\n" {
		t.Fatalf("marshal = %q", out)
	}
	var back []ModuleSpec
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != 2 || back[0] != in[0] || back[1] != in[1] {
		t.Errorf("round trip = %+v, want %+v", back, in)
	}
}

func TestFileSpecForms(t *testing.T) {
	const y = `
files:
  claude-settings.json: ~/.claude/settings.json
  secrets/auth.json:
    to: ~/.codex/auth.json
    mode: "0600"
`
	var s Spec
	if err := yaml.Unmarshal([]byte(y), &s); err != nil {
		t.Fatal(err)
	}
	if got := s.Files["claude-settings.json"]; got != (FileSpec{To: "~/.claude/settings.json"}) {
		t.Errorf("scalar form = %+v", got)
	}
	if got := s.Files["secrets/auth.json"]; got != (FileSpec{To: "~/.codex/auth.json", Mode: "0600"}) {
		t.Errorf("map form = %+v", got)
	}
}

func TestParseModuleRef(t *testing.T) {
	cases := map[string]ModuleSpec{
		"node":                  {Name: "node"},
		"node@lts":              {Name: "node", Version: "lts"},
		"go@1.24":               {Name: "go", Version: "1.24"},
		"npm:@openai/codex":     {Name: "npm:@openai/codex"},
		"npm:@openai/codex@1.2": {Name: "npm:@openai/codex", Version: "1.2"},
		"aqua:owner/repo":       {Name: "aqua:owner/repo"},
	}
	for in, want := range cases {
		if got := ParseModuleRef(in); got != want {
			t.Errorf("ParseModuleRef(%q) = %+v, want %+v", in, got, want)
		}
	}
}
