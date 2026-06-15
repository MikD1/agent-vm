package cli

import (
	"context"
	"errors"
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
		Name: "my-api", Source: "cli", User: "me",
		Modules:   []string{"node"},
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		Workspace: config.Workspace{Mode: "mount", GuestPath: "/home/me/my-api", HostPath: "/h/my-api"},
	}
	err := runCreate(context.Background(), deps, r, "/home/me", nowFixed())
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
	r := config.Resolved{Name: "my-api", Workspace: config.Workspace{Mode: "mount"}}
	if err := runCreate(context.Background(), deps, r, "/home/me", nowFixed()); err == nil {
		t.Error("create must refuse when a Record already exists")
	}
}

func nowFixed() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) }
