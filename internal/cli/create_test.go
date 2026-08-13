package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/registry"
)

// failRunner fails on the first provision (shell) call, succeeds otherwise, and
// records whether a delete (rollback) happened.
type failRunner struct {
	deleted bool
}

func (f *failRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	switch args[0] {
	case "shell":
		return nil, []byte("provision boom"), errors.New("provision failed")
	case "delete":
		f.deleted = true
	}
	return nil, nil, nil
}

func TestCreateRecordFirstThenRollback(t *testing.T) {
	root := t.TempDir()
	store := registry.NewStore(root)
	fr := &failRunner{}
	deps := createDeps{
		lima:  lima.New(fr),
		store: store,
	}
	r := config.Resolved{
		Name: "my-api", User: "me",
		Modules:   []config.ModuleSpec{{Name: "node"}},
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		Workspace: config.Mount{HostPath: "/h/my-api", GuestPath: "/home/me/my-api"},
	}
	err := runCreate(context.Background(), deps, r, "/home/me", nowFixed(), false)
	if err == nil {
		t.Fatal("want provisioning error")
	}
	// Record-first: the Record must remain (→ OrphanedRecord) after rollback.
	ok, _ := store.Exists("my-api")
	if !ok {
		t.Error("Record must be kept after provisioning failure (OrphanedRecord)")
	}
	if !fr.deleted {
		t.Error("VM artifact must be rolled back via limactl delete")
	}
}

func TestCreateRefusesExistingRecord(t *testing.T) {
	root := t.TempDir()
	store := registry.NewStore(root)
	_ = store.Write(registry.Record{Name: "my-api"})
	deps := createDeps{lima: lima.New(&failRunner{}), store: store}
	r := config.Resolved{Name: "my-api", Workspace: config.Mount{HostPath: "/h/my-api", GuestPath: "/home/me/my-api"}}
	if err := runCreate(context.Background(), deps, r, "/home/me", nowFixed(), false); err == nil {
		t.Error("create must refuse when a Record already exists")
	}
}

func nowFixed() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) }

func TestResolveMountInputs(t *testing.T) {
	specDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(specDir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "tool"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveMountInputs(
		[]config.MountSpec{{Path: "lib", Name: "shared"}},
		[]string{filepath.Join(cwd, "tool")},
		specDir, cwd,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 inputs, got %d (%+v)", len(got), got)
	}
	if got[0].HostPath != filepath.Join(specDir, "lib") || got[0].Name != "shared" {
		t.Errorf("spec mount resolved wrong: %+v", got[0])
	}
	if got[1].HostPath != filepath.Join(cwd, "tool") {
		t.Errorf("flag mount resolved wrong: %+v", got[1])
	}
}

func TestResolveMountInputsMissing(t *testing.T) {
	specDir := t.TempDir()
	if _, err := resolveMountInputs([]config.MountSpec{{Path: "nope"}}, nil, specDir, specDir); err == nil {
		t.Error("want error for a missing mount source")
	}
}

func TestResolveFileInputs(t *testing.T) {
	specDir := t.TempDir()
	store := t.TempDir()
	if err := os.WriteFile(filepath.Join(specDir, "settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(specDir, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "auth.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := resolveFileInputs(map[string]config.FileSpec{
		"settings.json":                   {To: "~/.claude/settings.json"},
		"agents":                          {To: "~/.claude/agents"},
		filepath.Join(store, "auth.json"): {To: "~/.codex/auth.json", Mode: "0600"},
	}, specDir, store)
	if err != nil {
		t.Fatal(err)
	}
	byRel := map[string]config.FileInput{}
	for _, in := range got {
		byRel[in.Rel] = in
	}
	if in := byRel["settings.json"]; in.Root != config.RootWorkspace || in.IsDir {
		t.Errorf("settings.json = %+v", in)
	}
	if in := byRel["agents"]; in.Root != config.RootWorkspace || !in.IsDir {
		t.Errorf("agents = %+v", in)
	}
	if in := byRel["auth.json"]; in.Root != config.RootSecrets || in.Mode != "0600" {
		t.Errorf("auth.json = %+v", in)
	}
}

func TestResolveScriptInputs(t *testing.T) {
	specDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(specDir, "provision"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(specDir, "provision", "docker.sh")
	if err := os.WriteFile(p, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveScriptInputs([]string{"provision/docker.sh"}, specDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != p {
		t.Errorf("resolveScriptInputs() = %v, want [%s]", got, p)
	}
	if _, err := resolveScriptInputs([]string{"nope.sh"}, specDir); err == nil {
		t.Error("missing script = nil error, want an error")
	}
}

// TestCreateCmdWorkspaceFlags pins the create surface: the workspace comes from
// the project directory, so no flag may introduce another source for it.
func TestCreateCmdWorkspaceFlags(t *testing.T) {
	cmd := newCreateCmd()
	for _, name := range []string{"repo", "ref"} {
		if f := cmd.Flags().Lookup(name); f != nil {
			t.Errorf("create must not define a --%s flag, got %q", name, f.Usage)
		}
	}
	if err := cmd.Flags().Parse([]string{"--repo=git@h:a/b.git"}); err == nil {
		t.Error("--repo must be rejected as an unknown flag")
	}
}

func TestResolveFileInputsRejects(t *testing.T) {
	specDir := t.TempDir()
	store := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "x.conf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "dir.conf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A source neither in the project nor in the host store is invisible to the guest.
	if _, err := resolveFileInputs(map[string]config.FileSpec{
		filepath.Join(outside, "x.conf"): {To: "~/x.conf"},
	}, specDir, store); err == nil {
		t.Error("source outside both roots = nil error, want an error")
	}
	// A missing source is caught before the VM is created.
	if _, err := resolveFileInputs(map[string]config.FileSpec{
		"nope.json": {To: "~/nope.json"},
	}, specDir, store); err == nil {
		t.Error("missing source = nil error, want an error")
	}
	// mode is meaningless for a directory.
	if err := os.MkdirAll(filepath.Join(specDir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveFileInputs(map[string]config.FileSpec{
		"d": {To: "~/d", Mode: "0600"},
	}, specDir, store); err == nil {
		t.Error("mode on a directory = nil error, want an error")
	}
}
