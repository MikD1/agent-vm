package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
		ConfigDir: "/Users/me/vms/work", Home: "/home/me",
	}
	_ = store.Write(rec)
	r := &okRunner{}
	deps := createDeps{lima: lima.New(r), store: store}
	if err := runRecreate(context.Background(), deps, "my-api", hostMacOS, false); err != nil {
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
	if err := runRecreate(context.Background(), deps, "ghost", hostMacOS, false); err == nil {
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
		ConfigDir: "/Users/me/vms/work", Home: "/home/me",
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

// TestRecreateDoesNotCallLimaInfo proves recreate takes the guest home from the
// Record — the one the VM was built with — instead of asking limactl, whose
// answer describes the template and can shift with the Lima version.
func TestRecreateDoesNotCallLimaInfo(t *testing.T) {
	store := registry.NewStore(t.TempDir())
	_ = store.Write(registry.Record{
		Name: "work", User: "me",
		Modules:   []config.ModuleSpec{{Name: "node"}},
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		ConfigDir: "/Users/me/vms/work",
		Home:      "/home/me.linux",
	})
	r := &okRunner{}
	deps := createDeps{lima: lima.New(r), store: store}
	if err := runRecreate(context.Background(), deps, "work", hostMacOS, false); err != nil {
		t.Fatal(err)
	}
	for _, op := range r.ops {
		if op == "info" {
			t.Errorf("recreate must not call `limactl info`; ops = %v", r.ops)
		}
	}
}

func TestRecordToResolvedCarriesMounts(t *testing.T) {
	rec := registry.Record{
		Name: "my-api", User: "me",
		ConfigDir: "/Users/me/vms/work", Home: "/home/me",
		Mounts: []config.Mount{{HostPath: "/h/shared", GuestPath: "/home/me/shared"}},
	}
	r := recordToResolved(rec)
	if len(r.Mounts) != 1 || r.Mounts[0].GuestPath != "/home/me/shared" {
		t.Errorf("recordToResolved dropped mounts: %+v", r.Mounts)
	}
}

// writeSpecDir creates a VM directory holding an agent-vm.yaml with the given body.
func writeSpecDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent-vm.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestRecreateRereadsSpec is the point of resolveForRecreate: the documented
// model is "change agent-vm.yaml, then recreate". A rebuild that replayed the
// Record's snapshot would silently ignore the edit that prompted it.
func TestRecreateRereadsSpec(t *testing.T) {
	dir := writeSpecDir(t, "modules:\n  - go: \"1.24\"\nresources: {cpus: 8}\n")
	store := registry.NewStore(t.TempDir())
	_ = store.Write(registry.Record{
		Name: "work", User: "me",
		Modules:   []config.ModuleSpec{{Name: "node", Version: "lts"}},
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		ConfigDir: dir, Home: "/home/me",
	})
	deps := createDeps{lima: lima.New(&okRunner{}), store: store}
	if err := runRecreate(context.Background(), deps, "work", hostMacOS, false); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read("work")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Modules) != 1 || got.Modules[0].Name != "go" || got.Modules[0].Version != "1.24" {
		t.Errorf("recreate must rebuild from the spec's modules; record has %+v", got.Modules)
	}
	if got.Resources.CPUs != 8 {
		t.Errorf("recreate must pick up the spec's resources; cpus = %d, want 8", got.Resources.CPUs)
	}
}

// TestRecreateKeepsRecordCreatedAt guards the write-back: recreating rebuilds an
// existing VM, so its creation stamp must survive.
func TestRecreateKeepsRecordCreatedAt(t *testing.T) {
	dir := writeSpecDir(t, "modules: [node]\n")
	store := registry.NewStore(t.TempDir())
	_ = store.Write(registry.Record{
		Name: "work", User: "me", CreatedAt: nowFixed(),
		Modules:   []config.ModuleSpec{{Name: "node"}},
		ConfigDir: dir, Home: "/home/me",
	})
	deps := createDeps{lima: lima.New(&okRunner{}), store: store}
	if err := runRecreate(context.Background(), deps, "work", hostMacOS, false); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Read("work")
	if !got.CreatedAt.Equal(nowFixed()) {
		t.Errorf("CreatedAt = %v, want it carried over unchanged (%v)", got.CreatedAt, nowFixed())
	}
}

// TestRecreateIgnoresSpecName proves a renamed `name:` key cannot redirect the
// rebuild at a different VM: recreate targets the name it was given.
func TestRecreateIgnoresSpecName(t *testing.T) {
	dir := writeSpecDir(t, "name: something-else\nmodules: [node]\n")
	store := registry.NewStore(t.TempDir())
	_ = store.Write(registry.Record{
		Name: "work", User: "me",
		Modules:   []config.ModuleSpec{{Name: "node"}},
		ConfigDir: dir, Home: "/home/me",
	})
	deps := createDeps{lima: lima.New(&okRunner{}), store: store}
	if err := runRecreate(context.Background(), deps, "work", hostMacOS, false); err != nil {
		t.Fatal(err)
	}
	if ok, _ := store.Exists("something-else"); ok {
		t.Error("the spec's name must not create a second record")
	}
	got, _ := store.Read("work")
	if got.Name != "work" {
		t.Errorf("record name = %q, want it unchanged", got.Name)
	}
}

// TestRecreateFallsBackToRecordWithoutSpec keeps the property architecture §5
// relies on: an orphaned Record can be recreated on a host that does not have
// the VM directory at all.
func TestRecreateFallsBackToRecordWithoutSpec(t *testing.T) {
	store := registry.NewStore(t.TempDir())
	_ = store.Write(registry.Record{
		Name: "work", User: "me",
		Modules:   []config.ModuleSpec{{Name: "node", Version: "lts"}},
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		ConfigDir: filepath.Join(t.TempDir(), "gone"), Home: "/home/me",
	})
	deps := createDeps{lima: lima.New(&okRunner{}), store: store}
	if err := runRecreate(context.Background(), deps, "work", hostMacOS, false); err != nil {
		t.Fatalf("recreate must work without the VM directory: %v", err)
	}
	got, _ := store.Read("work")
	if len(got.Modules) != 1 || got.Modules[0].Name != "node" {
		t.Errorf("without a spec the record's own modules must be used; got %+v", got.Modules)
	}
}

// TestRecreateReportsBadSpec: a spec that is present but broken is a mistake to
// surface, not something to paper over by falling back to the snapshot.
func TestRecreateReportsBadSpec(t *testing.T) {
	dir := writeSpecDir(t, "resources: {memory: enormous}\n")
	store := registry.NewStore(t.TempDir())
	_ = store.Write(registry.Record{
		Name: "work", User: "me", ConfigDir: dir, Home: "/home/me",
	})
	deps := createDeps{lima: lima.New(&okRunner{}), store: store}
	err := runRecreate(context.Background(), deps, "work", hostMacOS, false)
	if err == nil || !strings.Contains(err.Error(), "enormous") {
		t.Errorf("want the spec's validation error, got %v", err)
	}
}
