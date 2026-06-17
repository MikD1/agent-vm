package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/provision"
	"github.com/MikD1/agent-vm/internal/registry"
	"github.com/spf13/cobra"
)

// recordToResolved rebuilds the Resolved config from a stored Record.
func recordToResolved(rec registry.Record) config.Resolved {
	return config.Resolved{
		Name: rec.Name, Source: rec.Source, User: rec.User,
		Modules: rec.Modules, Resources: rec.Resources, Base: rec.Base, Workspace: rec.Workspace,
	}
}

// runRecreate reads the Record, deletes any existing VM, and rebuilds pristinely.
// The Record is NOT rewritten (it is the source of truth for recreation).
func runRecreate(ctx context.Context, deps createDeps, name string) error {
	rec, err := deps.store.Read(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no record for %q; nothing to recreate", name)
		}
		return fmt.Errorf("read record for %q: %w", name, err)
	}
	r := recordToResolved(rec)

	infoJSON, err := deps.lima.InfoRaw(ctx)
	if err != nil {
		return fmt.Errorf("limactl info: %w", err)
	}
	guestHomeVal, err := guestHome(infoJSON, rec.User)
	if err != nil {
		return err
	}

	limaYAML, err := buildLimaConfig(r, guestHomeVal)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "avm-"+name+"-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(limaYAML); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	_ = deps.lima.Delete(ctx, name) // pristine: remove any existing VM first
	p := provision.New(deps.lima, deps.externalDir)
	if provErr := p.Run(ctx, r, tmp.Name()); provErr != nil {
		rollbackMsg := "VM rolled back"
		if delErr := deps.lima.Delete(ctx, name); delErr != nil {
			rollbackMsg = fmt.Sprintf("VM rollback attempted but may have failed (%v); verify with `limactl list`", delErr)
		}
		return fmt.Errorf("%w\n%s; record kept. Run `avm recreate %s` to retry", provErr, rollbackMsg, name)
	}
	fmt.Printf("VM recreated: %s\n", name)
	return nil
}

func newRecreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "recreate <name>",
		Short: "Pristine rebuild of a VM from its record (clone mode re-clones — commit & push first)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			limaClient := newLimaClient(cmd)
			root, err := registry.DefaultRoot()
			if err != nil {
				return err
			}
			store := registry.NewStore(root)
			deps := createDeps{lima: limaClient, store: store, externalDir: externalModuleDir(root)}
			return runRecreate(ctx, deps, args[0])
		},
	}
}
