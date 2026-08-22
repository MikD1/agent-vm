package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MikD1/agent-vm/internal/templates"
	"github.com/MikD1/agent-vm/internal/vmname"
	"github.com/spf13/cobra"
)

func runInit(dir string, force bool) error {
	dest := filepath.Join(dir, "agent-vm.yaml")
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("directory not found: %s", dir)
		}
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		if !force {
			return fmt.Errorf("agent-vm.yaml already exists in %s (use --force to overwrite)", dir)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	name, err := initVMName(dir)
	if err != nil {
		return err
	}
	spec, err := templates.RenderSpec(name)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dest, spec, 0o644); err != nil {
		return err
	}
	fmt.Printf("Created %s\nEdit it to select modules and list your projects, then run: avm create\n", dest)
	return nil
}

// initVMName is the name the template ships with: the VM directory's own name,
// normalized the way `avm create` would normalize it anyway. Writing it out
// keeps the file explicit — the default is visible and editable, not implied.
func initVMName(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return vmname.Normalize(filepath.Base(abs))
}

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Write an agent-vm.yaml template",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return runInit(dir, force)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite an existing file")
	return cmd
}
