package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/spf13/cobra"
)

func cwd() string {
	d, err := os.Getwd()
	if err != nil {
		return "."
	}
	return d
}

func newShellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell [name]",
		Short: "Open a shell in the VM",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := ""
			if len(args) == 1 {
				arg = args[0]
			}
			name, err := resolveTargetName(arg, cwd())
			if err != nil {
				return err
			}
			c := newLimaClient(cmd)
			return c.Shell(cmd.Context(), name, "")
		},
	}
}

func lifecycleCmd(use, short string, fn func(*lima.Client, context.Context, string) error) *cobra.Command {
	return &cobra.Command{
		Use:   use + " [name]",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := ""
			if len(args) == 1 {
				arg = args[0]
			}
			name, err := resolveTargetName(arg, cwd())
			if err != nil {
				return err
			}
			if err := fn(newLimaClient(cmd), cmd.Context(), name); err != nil {
				return err
			}
			fmt.Printf("%s: %s\n", use, name)
			return nil
		},
	}
}

func newStartCmd() *cobra.Command {
	return lifecycleCmd("start", "Start a stopped VM", (*lima.Client).Start)
}
func newStopCmd() *cobra.Command {
	return lifecycleCmd("stop", "Stop a running VM", (*lima.Client).Stop)
}
func newRestartCmd() *cobra.Command {
	return lifecycleCmd("restart", "Restart a VM", (*lima.Client).Restart)
}
