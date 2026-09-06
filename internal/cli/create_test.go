package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		ConfigDir: "/Users/me/vms/work", Home: "/home/me",
	}
	err := runCreate(context.Background(), deps, r, "/home/me", hostMacOS, nowFixed(), false)
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
	r := config.Resolved{Name: "my-api", ConfigDir: "/Users/me/vms/work", Home: "/home/me"}
	if err := runCreate(context.Background(), deps, r, "/home/me", hostMacOS, nowFixed(), false); err == nil {
		t.Error("create must refuse when a Record already exists")
	}
}

func nowFixed() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) }

func TestResolveMountInputs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveMountInputs([]config.MountSpec{
		{Path: filepath.Join(dir, "lib"), Name: "shared"},
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 input, got %d (%+v)", len(got), got)
	}
	if got[0].HostPath != filepath.Join(dir, "lib") || got[0].Name != "shared" {
		t.Errorf("mount resolved wrong: %+v", got[0])
	}
}

func TestResolveMountInputsMissing(t *testing.T) {
	dir := t.TempDir()
	if _, err := resolveMountInputs([]config.MountSpec{{Path: filepath.Join(dir, "nope")}}, dir); err == nil {
		t.Error("want error for a missing mount source")
	}
}

// TestResolveMountInputsRejectsVMDir: declaring the VM's own directory (or a
// parent of it) under `mounts:` must fail `avm create`. Lima would otherwise
// apply both that read/write mount and the read-only service mount of the same
// host path, giving the guest write access to agent-vm.yaml and every
// credential declared in `files`.
func TestResolveMountInputsRejectsVMDir(t *testing.T) {
	parent := t.TempDir()
	configDir := filepath.Join(parent, "work")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{configDir, parent, configDir + string(filepath.Separator) + "."} {
		if _, err := resolveMountInputs([]config.MountSpec{{Path: path}}, configDir); err == nil {
			t.Errorf("mounting %q must be rejected", path)
		} else if !strings.Contains(err.Error(), "/mnt/host/vm") {
			t.Errorf("error = %q, want it to point at the read-only service mount", err)
		}
	}
	// A sibling directory is unaffected.
	sibling := filepath.Join(parent, "projects")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveMountInputs([]config.MountSpec{{Path: sibling}}, configDir); err != nil {
		t.Errorf("a sibling folder must still be mountable: %v", err)
	}
}

func TestResolveFileInputs(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveFileInputs(map[string]config.FileSpec{
		"settings.json": {To: "~/.claude/settings.json"},
		"agents":        {To: "~/.claude/agents"},
	}, configDir)
	if err != nil {
		t.Fatal(err)
	}
	byRel := map[string]config.FileInput{}
	for _, in := range got {
		byRel[in.Rel] = in
	}
	if in := byRel["settings.json"]; in.IsDir || in.To != "~/.claude/settings.json" {
		t.Errorf("settings.json = %+v", in)
	}
	if in := byRel["agents"]; !in.IsDir {
		t.Errorf("agents = %+v", in)
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

func TestVMName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "My_Work_VM")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No name in the spec → basename of the VM directory, normalized.
	got, err := vmName(config.Spec{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "my-work-vm" {
		t.Errorf("vmName(no name) = %q, want my-work-vm", got)
	}
	// An explicit name wins over the folder name, and is normalized too.
	got2, err := vmName(config.Spec{Name: "Work_Domain"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got2 != "work-domain" {
		t.Errorf("vmName(explicit) = %q, want work-domain", got2)
	}
}

// TestCreateCmdHasNoMountFlag pins decision 11: the config is the source of
// truth for mounts, so no flag may add one behind its back.
func TestCreateCmdHasNoMountFlag(t *testing.T) {
	cmd := newCreateCmd()
	if f := cmd.Flags().Lookup("mount"); f != nil {
		t.Errorf("create must not define a --mount flag, got %q", f.Usage)
	}
	if err := cmd.Flags().Parse([]string{"--mount=/tmp/x"}); err == nil {
		t.Error("--mount must be rejected as an unknown flag")
	}
}

func TestLoadSpecForCreateMissing(t *testing.T) {
	dir := t.TempDir()
	_, _, err := loadSpecForCreate(dir)
	if err == nil {
		t.Fatal("want an error when agent-vm.yaml is absent")
	}
	if !strings.Contains(err.Error(), "agent-vm.yaml not found") || !strings.Contains(err.Error(), "avm init") {
		t.Errorf("error = %q, want it to name the file and suggest avm init", err)
	}
	// The dotted spelling must not appear anywhere in the message.
	if strings.Contains(err.Error(), ".agent-vm.yaml") {
		t.Errorf("error uses the dotted filename: %q", err)
	}
}

func TestResolveFileInputsRejects(t *testing.T) {
	configDir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "x.conf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A source outside the VM directory is invisible to the guest.
	if _, err := resolveFileInputs(map[string]config.FileSpec{
		filepath.Join(outside, "x.conf"): {To: "~/x.conf"},
	}, configDir); err == nil {
		t.Error("source outside the VM directory = nil error, want an error")
	}
	// A missing source is caught before the VM is created.
	if _, err := resolveFileInputs(map[string]config.FileSpec{
		"nope.json": {To: "~/nope.json"},
	}, configDir); err == nil {
		t.Error("missing source = nil error, want an error")
	}
	// mode is meaningless for a directory.
	if err := os.MkdirAll(filepath.Join(configDir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveFileInputs(map[string]config.FileSpec{
		"d": {To: "~/d", Mode: "0600"},
	}, configDir); err == nil {
		t.Error("mode on a directory = nil error, want an error")
	}
}
