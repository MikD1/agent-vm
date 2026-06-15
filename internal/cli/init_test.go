package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunInitWritesTemplate(t *testing.T) {
	dir := t.TempDir()
	if err := runInit(dir, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".agent-vm.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("template is empty")
	}
	// second call without force fails
	if err := runInit(dir, false); err == nil {
		t.Error("want error when file exists and force=false")
	}
	// with force succeeds
	if err := runInit(dir, true); err != nil {
		t.Errorf("force overwrite failed: %v", err)
	}
}
