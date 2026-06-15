package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/registry"
)

// okRunner records ops and returns empty success for all calls.
type okRunner struct{ ops []string }

func (o *okRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	o.ops = append(o.ops, args[0])
	if args[0] == "list" {
		return []byte(""), nil, nil
	}
	return nil, nil, nil
}

func TestRunDeleteRemovesVMAndRecord(t *testing.T) {
	store := registry.NewStore(t.TempDir())
	_ = store.Write(registry.Record{Name: "my-api"})
	r := &okRunner{}
	if err := runDelete(context.Background(), lima.New(r), store, "my-api"); err != nil {
		t.Fatal(err)
	}
	ok, _ := store.Exists("my-api")
	if ok {
		t.Error("record must be removed by delete")
	}
}

// namesRunner returns a fixed list of Lima VM names for reconciliation.
type namesRunner struct{ names []string }

func (n *namesRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	if args[0] == "list" {
		return []byte(strings.Join(n.names, "\n")), nil, nil
	}
	return nil, nil, nil
}

func stubNames(names []string) *lima.Client {
	return lima.New(&namesRunner{names: names})
}

func TestRunPruneRemovesOrphansOnly(t *testing.T) {
	store := registry.NewStore(t.TempDir())
	_ = store.Write(registry.Record{Name: "managed"})
	_ = store.Write(registry.Record{Name: "orphan"})
	pruned, err := runPrune(context.Background(), stubNames([]string{"managed"}), store, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned) != 1 || pruned[0] != "orphan" {
		t.Errorf("pruned = %v, want [orphan]", pruned)
	}
	if ok, _ := store.Exists("managed"); !ok {
		t.Error("managed record must survive prune")
	}
}
