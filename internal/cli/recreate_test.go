package cli

import (
	"context"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/registry"
)

func TestRecreateFromRecord(t *testing.T) {
	store := registry.NewStore(t.TempDir())
	rec := registry.Record{
		Name: "my-api", User: "me",
		Modules:   []config.ModuleSpec{{Name: "node"}},
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		Workspace: config.Workspace{Mode: "mount", GuestPath: "/home/me/my-api", HostPath: "/h/my-api"},
	}
	_ = store.Write(rec)
	r := &okRunner{}
	deps := createDeps{lima: lima.New(r), store: store}
	if err := runRecreate(context.Background(), deps, "my-api", false); err != nil {
		t.Fatal(err)
	}
	if ok, _ := store.Exists("my-api"); !ok {
		t.Error("record must survive recreate")
	}
	saw := map[string]bool{}
	for _, op := range r.ops {
		saw[op] = true
	}
	if !saw["delete"] || !saw["create"] || !saw["start"] {
		t.Errorf("recreate should delete then create+start; ops=%v", r.ops)
	}
}

func TestRecreateMissingRecord(t *testing.T) {
	store := registry.NewStore(t.TempDir())
	deps := createDeps{lima: lima.New(&okRunner{}), store: store}
	if err := runRecreate(context.Background(), deps, "ghost", false); err == nil {
		t.Error("want error recreating a VM with no record")
	}
}

// TestRecordToResolvedBackfillsEmptyVersion guards a Record written before
// per-tool versions existed (or hand-edited from the old `modules: [name]`
// shorthand), where a ModuleSpec's Version is "". recordToResolved bypasses
// config.Resolve — the only place DefaultToolVersion is normally applied — so
// it must backfill the empty Version itself; otherwise renderMiseConfig would
// emit a broken `"node" = ""` entry into the guest's mise config.
func TestRecordToResolvedBackfillsEmptyVersion(t *testing.T) {
	rec := registry.Record{
		Name: "my-api", User: "me",
		Modules:   []config.ModuleSpec{{Name: "node", Version: ""}, {Name: "go", Version: "1.24"}},
		Workspace: config.Workspace{Mode: "mount", GuestPath: "/home/me/my-api", HostPath: "/h/my-api"},
	}
	r := recordToResolved(rec)
	if len(r.Modules) != 2 {
		t.Fatalf("recordToResolved(%+v).Modules = %+v, want 2 entries", rec.Modules, r.Modules)
	}
	if r.Modules[0].Name != "node" || r.Modules[0].Version != config.DefaultToolVersion {
		t.Errorf("empty Version not backfilled: got %+v, want Version %q", r.Modules[0], config.DefaultToolVersion)
	}
	if r.Modules[1].Version != "1.24" {
		t.Errorf("an already-set Version must not be overwritten: got %+v", r.Modules[1])
	}
	// The Record itself (rec.Modules) must not be mutated in place.
	if rec.Modules[0].Version != "" {
		t.Errorf("recordToResolved must not mutate the input Record: rec.Modules = %+v", rec.Modules)
	}
}

func TestRecordToResolvedCarriesMounts(t *testing.T) {
	rec := registry.Record{
		Name: "my-api", User: "me",
		Workspace: config.Workspace{Mode: "mount", GuestPath: "/home/me/my-api", HostPath: "/h/my-api"},
		Mounts:    []config.Mount{{HostPath: "/h/shared", GuestPath: "/home/me/shared"}},
	}
	r := recordToResolved(rec)
	if len(r.Mounts) != 1 || r.Mounts[0].GuestPath != "/home/me/shared" {
		t.Errorf("recordToResolved dropped mounts: %+v", r.Mounts)
	}
}
