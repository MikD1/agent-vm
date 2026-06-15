package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/registry"
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
		Short: "Open a shell in the VM (at the workspace dir)",
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
			c := lima.New(lima.ExecRunner{})
			workdir := workspaceDir(name)
			return c.Shell(cmd.Context(), name, workdir)
		},
	}
}

// workspaceDir returns the guest workspace path from the Record, or "" if none.
func workspaceDir(name string) string {
	root, err := registry.DefaultRoot()
	if err != nil {
		return ""
	}
	rec, err := registry.NewStore(root).Read(name)
	if err != nil {
		return ""
	}
	return rec.Workspace.GuestPath
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
			if err := fn(lima.New(lima.ExecRunner{}), cmd.Context(), name); err != nil {
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
