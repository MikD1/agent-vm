package modules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedScriptsPresent(t *testing.T) {
	for _, m := range []string{"system", "base", "node", "dotnet", "go", "docker", "claude", "codex"} {
		b, err := Script(m, "")
		if err != nil {
			t.Errorf("Script(%q): %v", m, err)
		}
		if len(b) == 0 {
			t.Errorf("Script(%q): empty", m)
		}
	}
}

func TestExternalOverride(t *testing.T) {
	dir := t.TempDir()
	custom := []byte("#!/usr/bin/env bash\necho custom\n")
	if err := os.WriteFile(filepath.Join(dir, "node.sh"), custom, 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Script("node", dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(custom) {
		t.Error("external dir should override embedded module")
	}
}

func TestUnknownModule(t *testing.T) {
	if Exists("bogus", "") {
		t.Error("bogus should not exist")
	}
	if _, err := Script("../etc/passwd", ""); err == nil {
		t.Error("invalid name must be rejected")
	}
}
