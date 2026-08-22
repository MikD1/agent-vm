package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/specedit"
)

func TestRunInitWritesTemplate(t *testing.T) {
	dir := t.TempDir()
	if err := runInit(dir, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "agent-vm.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("template is empty")
	}
	// The name is written out, not left to be inferred from the folder.
	want := "name: " + filepath.Base(dir) + "\n"
	if !strings.Contains(string(data), want) {
		t.Errorf("template does not carry %q:\n%s", want, data)
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

// TestRunInitDoesNotWriteDottedFile guards the rename: the config is the main
// inhabitant of its own folder, not a hidden guest in someone else's repo.
func TestRunInitDoesNotWriteDottedFile(t *testing.T) {
	dir := t.TempDir()
	if err := runInit(dir, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agent-vm.yaml")); err == nil {
		t.Error("init must not write a dotted .agent-vm.yaml")
	}
}

// TestRunInitNormalizesTheName: the written name must be usable as-is, so a
// folder whose name is not a DNS label is normalized the way `avm create` would.
func TestRunInitNormalizesTheName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "My_Project")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runInit(dir, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "agent-vm.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "name: my-project\n") {
		t.Errorf("name was not normalized:\n%s", data)
	}
}

// TestRunInitWritesAMountableFile: the mounts section ships with only a
// commented example, and `avm mount` must still be able to append into it.
func TestRunInitWritesAMountableFile(t *testing.T) {
	dir := t.TempDir()
	if err := runInit(dir, false); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "agent-vm.yaml")
	if err := specedit.AddMount(p, config.MountSpec{Path: "~/projects/api"}); err != nil {
		t.Fatal(err)
	}
	s, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Mounts) != 1 || s.Mounts[0].Path != "~/projects/api" {
		t.Fatalf("mount was not appended: %+v", s.Mounts)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# - ~/projects") {
		t.Errorf("the commented example was lost:\n%s", data)
	}
}
