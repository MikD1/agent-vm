package templates

import (
	"strings"
	"testing"
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
