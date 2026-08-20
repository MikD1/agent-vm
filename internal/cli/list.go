package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/registry"
	"github.com/MikD1/agent-vm/internal/vmname"
	"github.com/spf13/cobra"
)

func formatList(entries []registry.Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-24s %-10s %-10s %s\n", "NAME", "STATUS", "STATE", "MOUNTS")
	for _, e := range entries {
		mounts := "-"
		if e.Record != nil && len(e.Record.Mounts) > 0 {
			mounts = fmt.Sprintf("+%d", len(e.Record.Mounts))
		}
		fmt.Fprintf(&b, "%-24s %-10s %-10s %s\n", e.Name, e.Status, e.State, mounts)
	}
	return b.String()
}

// formatDetail renders one VM as a block: its identity and resources, then the
// projects mounted into it and the tools mise resolved.
func formatDetail(e registry.Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s %s\n", "NAME", e.Name)
	fmt.Fprintf(&b, "%-10s %s\n", "STATUS", e.Status)
	fmt.Fprintf(&b, "%-10s %s\n", "STATE", e.State)
	if e.Record == nil {
		return b.String()
	}
	r := e.Record
	fmt.Fprintf(&b, "%-10s %s\n", "BASE", r.Base.Image)
	fmt.Fprintf(&b, "%-10s cpus %d, memory %s, disk %s\n", "RESOURCES",
		r.Resources.CPUs, r.Resources.Memory, r.Resources.Disk)
	fmt.Fprintf(&b, "%-10s %s\n", "CONFIG", r.ConfigDir)
	fmt.Fprintf(&b, "%-10s %s\n", "CREATED", r.CreatedAt.Format("2006-01-02 15:04:05"))

	b.WriteString("\nMOUNTS\n")
	if len(r.Mounts) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, m := range r.Mounts {
		fmt.Fprintf(&b, "  %s → %s\n", m.HostPath, m.GuestPath)
	}

	b.WriteString("\nTOOLS\n")
	if len(r.InstalledTools) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, t := range r.InstalledTools {
		fmt.Fprintf(&b, "  %s %s\n", t.Name, t.Version)
	}
	return b.String()
}

// runList renders the whole table, or one VM's detail block when name is set.
func runList(ctx context.Context, c *lima.Client, store *registry.Store, name string) (string, error) {
	records, err := store.List()
	if err != nil {
		return "", err
	}
	instances, err := c.Instances(ctx)
	if err != nil {
		return "", err
	}
	limaStates := make(map[string]string, len(instances))
	for _, inst := range instances {
		limaStates[inst.Name] = inst.State
	}
	entries := registry.ReconcileStates(records, limaStates)
	if name == "" {
		return formatList(entries), nil
	}
	for _, e := range entries {
		if e.Name == name {
			return formatDetail(e), nil
		}
	}
	return "", fmt.Errorf("no VM named %q", name)
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [name]",
		Short: "List VMs, or show one VM in detail",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// A VM is always recorded under its normalized name, so the argument
			// goes through the same normalization the lifecycle commands apply.
			name := ""
			if len(args) == 1 {
				n, err := vmname.Normalize(args[0])
				if err != nil {
					return err
				}
				name = n
			}
			root, err := registry.DefaultRoot()
			if err != nil {
				return err
			}
			out, err := runList(cmd.Context(), newLimaClient(cmd), registry.NewStore(root), name)
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
}
