package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/registry"
	"github.com/spf13/cobra"
)

func formatList(entries []registry.Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-24s %s\n", "NAME", "STATUS")
	for _, e := range entries {
		fmt.Fprintf(&b, "%-24s %s\n", e.Name, e.Status)
	}
	return b.String()
}

func runList(ctx context.Context, c *lima.Client, store *registry.Store) (string, error) {
	records, err := store.List()
	if err != nil {
		return "", err
	}
	names, err := c.Names(ctx)
	if err != nil {
		return "", err
	}
	return formatList(registry.Reconcile(records, names)), nil
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List VMs (managed / orphaned / unmanaged)",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := registry.DefaultRoot()
			if err != nil {
				return err
			}
			out, err := runList(cmd.Context(), newLimaClient(cmd), registry.NewStore(root))
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
}
