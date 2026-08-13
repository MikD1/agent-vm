package templates

import (
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
	"gopkg.in/yaml.v3"
)

func TestEmbeddedTemplatesPresent(t *testing.T) {
	if !strings.Contains(string(BaseLima), "/mnt/host/agent-vm") {
		t.Error("base.yaml must mount the renamed host store")
	}
	if strings.Contains(string(BaseLima), "ai-dev-vm") {
		t.Error("base.yaml still references the old name")
	}
	if !strings.Contains(string(SpecTemplate), "modules:") {
		t.Error("spec template must list modules")
	}
}

func TestSpecTemplateDocumentsMounts(t *testing.T) {
	if !strings.Contains(string(SpecTemplate), "mounts:") {
		t.Error("spec template should show a mounts example")
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
