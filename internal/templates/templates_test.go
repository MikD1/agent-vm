package templates

import (
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
	"gopkg.in/yaml.v3"
)

func TestEmbeddedTemplatesPresent(t *testing.T) {
	if strings.Contains(string(BaseLima), "ai-dev-vm") {
		t.Error("base.yaml still references the old name")
	}
	if !strings.Contains(string(SpecTemplate), "modules:") {
		t.Error("spec template must list modules")
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
	if !strings.Contains(string(SpecTemplate), "\nmounts:") {
		t.Error("spec template must carry a live mounts key")
	}
	var s config.Spec
	if err := yaml.Unmarshal(SpecTemplate, &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Mounts) != 0 {
		t.Errorf("template must declare no actual mounts, got %+v", s.Mounts)
	}
}

func TestSpecTemplateParsesAsASpec(t *testing.T) {
	var s config.Spec
	if err := yaml.Unmarshal(SpecTemplate, &s); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("template is not a valid Spec: %v", err)
	}
	if s.Modules == nil {
		t.Fatal("template declares no modules")
	}
	var node config.ModuleSpec
	for _, m := range *s.Modules {
		if m.Name == "node" {
			node = m
		}
	}
	if node.Version != "lts" {
		t.Errorf("template node version = %q, want \"lts\" (the LTS opinion lives in the template)", node.Version)
	}
	// Docker is platform, not a module.
	for _, m := range *s.Modules {
		if m.Name == "docker" {
			t.Error("template lists docker as a module; it is installed unconditionally")
		}
	}
}
