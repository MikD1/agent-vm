package templates

import (
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
	"gopkg.in/yaml.v3"
)

// renderSpec is the template as `avm init` writes it, for a VM named "work".
func renderSpec(t *testing.T) []byte {
	t.Helper()
	b, err := RenderSpec("work")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEmbeddedTemplatesPresent(t *testing.T) {
	if strings.Contains(string(BaseLima), "ai-dev-vm") {
		t.Error("base.yaml still references the old name")
	}
	if !strings.Contains(string(renderSpec(t)), "modules:") {
		t.Error("spec template must list modules")
	}
}

// TestRenderSpecFillsInTheName: `avm init` ships a file with the name already
// filled in, so nothing in it has to be uncommented before `avm create`.
func TestRenderSpecFillsInTheName(t *testing.T) {
	out := string(renderSpec(t))
	if !strings.Contains(out, "name: work\n") {
		t.Errorf("rendered spec does not set the name:\n%s", out)
	}
	if strings.Contains(out, "{{") {
		t.Errorf("rendered spec still holds a placeholder:\n%s", out)
	}
	var s config.Spec
	if err := yaml.Unmarshal([]byte(out), &s); err != nil {
		t.Fatal(err)
	}
	if s.Name != "work" {
		t.Errorf("spec name = %q, want \"work\"", s.Name)
	}
}

// TestSpecTemplateOffersAMountExample keeps one commented path in the mounts
// section: a shape to copy, without declaring a mount the user did not ask for.
func TestSpecTemplateOffersAMountExample(t *testing.T) {
	if !strings.Contains(string(renderSpec(t)), "# - ~/projects") {
		t.Error("mounts section must carry a commented example path")
	}
}

// TestBaseTemplateHasNoMounts: every mount is per-VM (the service mount points
// at that VM's own directory), so the static template must declare none.
func TestBaseTemplateHasNoMounts(t *testing.T) {
	var doc map[string]any
	if err := yaml.Unmarshal(BaseLima, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["mounts"]; ok {
		t.Errorf("base.yaml must not declare a mounts key:\n%s", BaseLima)
	}
}

// TestSpecTemplateMountsSectionIsLive proves `avm init` writes a mounts key that
// specedit.AddMount can append into, while still parsing as "no mounts yet" so
// `avm create` works before any project is attached.
func TestSpecTemplateMountsSectionIsLive(t *testing.T) {
	if !strings.Contains(string(renderSpec(t)), "\nmounts:") {
		t.Error("spec template must carry a live mounts key")
	}
	var s config.Spec
	if err := yaml.Unmarshal(renderSpec(t), &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Mounts) != 0 {
		t.Errorf("template must declare no actual mounts, got %+v", s.Mounts)
	}
}

func TestSpecTemplateParsesAsASpec(t *testing.T) {
	var s config.Spec
	if err := yaml.Unmarshal(renderSpec(t), &s); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("template is not a valid Spec: %v", err)
	}
	if s.Modules == nil {
		t.Fatal("template declares no modules")
	}
	var node config.ModuleSpec
	names := make([]string, 0, len(*s.Modules))
	for _, m := range *s.Modules {
		names = append(names, m.Name)
		if m.Name == "node" {
			node = m
		}
	}
	if got, want := strings.Join(names, ","), "node,claude,codex"; got != want {
		t.Errorf("template modules = %q, want %q (the agent tools come preselected)", got, want)
	}
	if node.Version != "lts" {
		t.Errorf("template node version = %q, want \"lts\" (the LTS opinion lives in the template)", node.Version)
	}
	// files: and scripts: ship as bare keys. Neither is a pointer in the Spec,
	// so an empty section is indistinguishable from an absent one — the sections
	// are there to be filled in, and declare nothing until they are.
	if len(s.Files) != 0 || len(s.Scripts) != 0 {
		t.Errorf("template must declare no files/scripts, got %v / %v", s.Files, s.Scripts)
	}
	// Docker is platform, not a module.
	for _, m := range *s.Modules {
		if m.Name == "docker" {
			t.Error("template lists docker as a module; it is installed unconditionally")
		}
	}
}
