package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/modules"
	"github.com/MikD1/agent-vm/internal/provision"
	"github.com/MikD1/agent-vm/internal/registry"
	"github.com/MikD1/agent-vm/internal/vmname"
	"github.com/spf13/cobra"
)

// createDeps are the injectable dependencies of runCreate (real or faked in tests).
type createDeps struct {
	lima        *lima.Client
	store       *registry.Store
	externalDir string
}

// runCreate performs Record-first creation: refuse on existing Record, write the
// Record, build the VM, and on any provisioning failure roll the VM back while
// keeping the Record (→ OrphanedRecord).
func runCreate(ctx context.Context, deps createDeps, r config.Resolved, guestHome string, now time.Time) error {
	exists, err := deps.store.Exists(r.Name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("a record for %q already exists; use `avm recreate %s` to rebuild or `avm prune %s` to discard", r.Name, r.Name, r.Name)
	}

	// Render the Lima config to a temp file.
	limaYAML, err := buildLimaConfig(r, guestHome)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "avm-"+r.Name+"-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(limaYAML); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	// Record-first.
	if err := deps.store.Write(registry.FromResolved(r, now)); err != nil {
		return err
	}

	// Build the VM. On failure: roll the VM artifact back, KEEP the Record.
	p := provision.New(deps.lima, deps.externalDir)
	if provErr := p.Run(ctx, r, tmp.Name()); provErr != nil {
		rollbackMsg := "VM rolled back"
		if delErr := deps.lima.Delete(ctx, r.Name); delErr != nil {
			rollbackMsg = fmt.Sprintf("VM rollback attempted but may have failed (%v); verify with `limactl list`", delErr)
		}
		return fmt.Errorf("%w\n%s; record kept. Run `avm recreate %s` to retry or `avm prune %s` to discard", provErr, rollbackMsg, r.Name, r.Name)
	}
	fmt.Printf("VM ready: %s\nConnect: avm shell %s\n", r.Name, r.Name)
	return nil
}

func newCreateCmd() *cobra.Command {
	var f config.Flags
	cmd := &cobra.Command{
		Use:   "create [path]",
		Short: "Create and start a VM (mount mode; --repo for clone mode)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			f.ModulesSet = cmd.Flags().Changed("modules")

			absDir, err := os.Getwd()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				absDir, err = filepath.Abs(args[0])
				if err != nil {
					return err
				}
			}

			limaClient := newLimaClient(cmd)

			projName, err := projectName(f, absDir)
			if err != nil {
				return err
			}
			user := deriveGuestUser(osUsername())
			infoJSON, err := limaClient.InfoRaw(ctx)
			if err != nil {
				return err
			}
			home, err := guestHome(infoJSON, user)
			if err != nil {
				return err
			}

			root, err := registry.DefaultRoot()
			if err != nil {
				return err
			}
			extDir := externalModuleDir(root)

			spec, specPresent, hostPath, err := loadSpecForCreate(f, absDir)
			if err != nil {
				return err
			}
			known := func(m string) bool { return modules.Exists(m, extDir) }
			if err := spec.Validate(known); err != nil {
				return err
			}

			env := config.Env{
				ProjectName: projName,
				GuestUser:   user,
				GuestHome:   home,
				HostPath:    hostPath,
				SpecPresent: specPresent,
			}
			resolved, err := config.Resolve(f, spec, env)
			if err != nil {
				return err
			}
			if err := vmname.Validate(resolved.Name); err != nil {
				return err
			}

			deps := createDeps{
				lima:        limaClient,
				store:       registry.NewStore(root),
				externalDir: extDir,
			}
			return runCreate(ctx, deps, resolved, home, time.Now())
		},
	}
	cmd.Flags().StringSliceVar(&f.Modules, "modules", nil, "feature modules in order (a,b,c)")
	cmd.Flags().IntVar(&f.CPUs, "cpus", 0, "override cpus")
	cmd.Flags().StringVar(&f.Memory, "memory", "", "override memory (e.g. 16GiB)")
	cmd.Flags().StringVar(&f.Disk, "disk", "", "override disk (e.g. 200GiB)")
	cmd.Flags().StringVar(&f.BaseImage, "base-image", "", "override base image")
	cmd.Flags().StringVar(&f.Repo, "repo", "", "clone mode: git repo URL")
	cmd.Flags().StringVar(&f.Ref, "ref", "", "clone mode: git ref (default main)")
	return cmd
}
