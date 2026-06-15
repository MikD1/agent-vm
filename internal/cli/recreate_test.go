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
		Modules:   []string{"node"},
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		Workspace: config.Workspace{Mode: "mount", GuestPath: "/home/me/my-api", HostPath: "/h/my-api"},
	}
	_ = store.Write(rec)
	r := &okRunner{}
	deps := createDeps{lima: lima.New(r), store: store}
	if err := runRecreate(context.Background(), deps, "my-api", "/home/me"); err != nil {
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
	if err := runRecreate(context.Background(), deps, "ghost", "/home/me"); err == nil {
		t.Error("want error recreating a VM with no record")
	}
}
