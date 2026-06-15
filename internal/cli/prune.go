package cli

import (
	"context"
	"fmt"

	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/registry"
	"github.com/spf13/cobra"
)

// runPrune removes orphaned records (Record without VM). If name != "", only that
// record is pruned (and only if orphaned). Returns the pruned names.
func runPrune(ctx context.Context, c *lima.Client, store *registry.Store, name string) ([]string, error) {
	records, err := store.List()
	if err != nil {
		return nil, err
	}
	names, err := c.Names(ctx)
	if err != nil {
		return nil, err
	}
	var pruned []string
	for _, e := range registry.Reconcile(records, names) {
		if e.Status != registry.StatusOrphaned {
			continue
		}
		if name != "" && e.Name != name {
			continue
		}
		if err := store.Delete(e.Name); err != nil {
			return pruned, err
		}
		pruned = append(pruned, e.Name)
	}
	return pruned, nil
}

func newPruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune [name]",
		Short: "Remove orphaned records (record without a VM)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			root, err := registry.DefaultRoot()
			if err != nil {
				return err
			}
			pruned, err := runPrune(cmd.Context(), lima.New(lima.ExecRunner{}), registry.NewStore(root), name)
			if err != nil {
				return err
			}
			if len(pruned) == 0 {
				fmt.Println("No orphaned records to prune.")
				return nil
			}
			for _, n := range pruned {
				fmt.Printf("Pruned: %s\n", n)
			}
			return nil
		},
	}
}
