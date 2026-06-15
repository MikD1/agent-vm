package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MikD1/agent-vm/internal/templates"
	"github.com/spf13/cobra"
)

func runInit(dir string, force bool) error {
	dest := filepath.Join(dir, ".agent-vm.yaml")
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("directory not found: %s", dir)
		}
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		if !force {
			return fmt.Errorf(".agent-vm.yaml already exists in %s (use --force to overwrite)", dir)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(dest, templates.SpecTemplate, 0o644); err != nil {
		return err
	}
	fmt.Printf("Created %s\nEdit it to select modules, then run: avm create\n", dest)
	return nil
}

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Write a .agent-vm.yaml template",
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
