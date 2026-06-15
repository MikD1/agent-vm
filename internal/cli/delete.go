package cli

import (
	"context"
	"fmt"

	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/registry"
	"github.com/spf13/cobra"
)

// runDelete stops+deletes the VM and removes its Record.
func runDelete(ctx context.Context, c *lima.Client, store *registry.Store, name string) error {
	_ = c.Stop(ctx, name)   // best-effort
	_ = c.Delete(ctx, name) // force delete; ignore "absent"
	return store.Delete(name)
}

func newDeleteCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Stop and delete a VM and remove its record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !force {
				fmt.Printf("Delete VM %q and its record? This is irreversible. [y/N] ", name)
				var reply string
				fmt.Scanln(&reply)
				if reply != "y" && reply != "Y" {
					fmt.Println("Aborted.")
					return nil
				}
			}
			root, err := registry.DefaultRoot()
			if err != nil {
				return err
			}
			if err := runDelete(cmd.Context(), lima.New(lima.ExecRunner{}), registry.NewStore(root), name); err != nil {
				return err
			}
			fmt.Printf("Deleted: %s\n", name)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation")
	return cmd
}
