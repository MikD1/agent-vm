package cli

import (
	"context"
	"testing"

	"github.com/MikD1/agent-vm/internal/registry"
)

// The shell must open at the guest home from the Record: without an explicit
// workdir, limactl tries the host cwd and host home, neither of which exists in
// the guest.
func TestRunShellOpensAtGuestHome(t *testing.T) {
	store := registry.NewStore(t.TempDir())
	if err := store.Write(registry.Record{Name: "main", Home: "/home/mik"}); err != nil {
		t.Fatal(err)
	}
	var gotName, gotWorkdir string
	sh := func(_ context.Context, name, workdir string) error {
		gotName, gotWorkdir = name, workdir
		return nil
	}
	if err := runShell(context.Background(), sh, store, "main"); err != nil {
		t.Fatal(err)
	}
	if gotName != "main" || gotWorkdir != "/home/mik" {
		t.Errorf("shell(%q, %q), want (main, /home/mik)", gotName, gotWorkdir)
	}
}

// A VM avm does not manage has no Record; Lima's own default is all that is
// left, so no workdir is forced.
func TestRunShellUnmanagedVMKeepsLimaDefault(t *testing.T) {
	store := registry.NewStore(t.TempDir())
	var gotWorkdir string
	sh := func(_ context.Context, _, workdir string) error {
		gotWorkdir = workdir
		return nil
	}
	if err := runShell(context.Background(), sh, store, "stray"); err != nil {
		t.Fatal(err)
	}
	if gotWorkdir != "" {
		t.Errorf("workdir = %q, want empty for a VM with no record", gotWorkdir)
	}
}

// An old Record written before `home` was stored gets the same treatment.
func TestShellWorkdirEmptyHome(t *testing.T) {
	store := registry.NewStore(t.TempDir())
	if err := store.Write(registry.Record{Name: "old"}); err != nil {
		t.Fatal(err)
	}
	if got := shellWorkdir(store, "old"); got != "" {
		t.Errorf("shellWorkdir = %q, want empty", got)
	}
}
