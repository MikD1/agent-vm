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

// shellWorkdir is where `avm shell` opens: the guest home, as recorded at
// create time. Passing it explicitly is what keeps the shell quiet — given no
// --workdir, limactl builds `cd <host cwd> || cd <host home>`, and neither host
// path exists in the guest, so opening a shell printed two
// `cd: No such file or directory` errors before dropping the user at ~ anyway.
//
// An empty result (a VM with no Record, or a Record written before `home` was
// stored) leaves the decision to Lima: there is nothing better to guess.
func shellWorkdir(store *registry.Store, name string) string {
	if store == nil {
		return ""
	}
	rec, err := store.Read(name)
	if err != nil {
		return ""
	}
	return rec.Home
}

// shellFunc opens an interactive shell in a VM at workdir. `lima.Client.Shell`
// is the production implementation; it is wrapped in this narrower type so the
// workdir decision can be tested without a real `limactl`.
type shellFunc func(ctx context.Context, name, workdir string) error

// runShell opens the interactive shell at the VM's guest home.
func runShell(ctx context.Context, sh shellFunc, store *registry.Store, name string) error {
	return sh(ctx, name, shellWorkdir(store, name))
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
			var store *registry.Store
			if root, err := registry.DefaultRoot(); err == nil {
				store = registry.NewStore(root)
			}
			c := newLimaClient(cmd)
			sh := func(ctx context.Context, name, workdir string) error {
				return c.Shell(ctx, name, workdir)
			}
			return runShell(cmd.Context(), sh, store, name)
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
