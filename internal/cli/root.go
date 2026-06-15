package cli

import "github.com/spf13/cobra"

// Version is overridable at build time via -ldflags.
var Version = "dev"

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "avm",
		Short:         "Isolated Lima dev VMs, one per project",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newInitCmd())
	root.AddCommand(newCreateCmd())
	root.AddCommand(newListCmd(), newDeleteCmd(), newPruneCmd(), newRecreateCmd())
	root.AddCommand(newShellCmd(), newStartCmd(), newStopCmd(), newRestartCmd())
	return root
}

// Execute builds the root command and runs it.
func Execute() error {
	return NewRootCmd().Execute()
}
