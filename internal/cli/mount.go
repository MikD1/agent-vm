package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/registry"
	"github.com/MikD1/agent-vm/internal/specedit"
	"github.com/MikD1/agent-vm/internal/vmname"
	"github.com/spf13/cobra"
)

// loadVMSpec reads a Record and locates its VM Spec, rejecting anything the
// mount commands cannot safely edit. The ConfigDir guard matters most: an empty
// value would make filepath.Join below resolve to the process's working
// directory, and specedit would then rewrite whatever agent-vm.yaml happens to
// sit there.
func loadVMSpec(deps createDeps, vmName string) (registry.Record, string, error) {
	rec, err := deps.store.Read(vmName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return registry.Record{}, "", fmt.Errorf("no VM named %q", vmName)
		}
		return registry.Record{}, "", fmt.Errorf("read record for %q: %w", vmName, err)
	}
	if rec.ConfigDir == "" || !filepath.IsAbs(rec.ConfigDir) || rec.Home == "" {
		return registry.Record{}, "", fmt.Errorf("record for %q has no usable VM directory; recreate the VM", vmName)
	}
	specPath := filepath.Join(rec.ConfigDir, "agent-vm.yaml")
	if _, err := os.Stat(specPath); err != nil {
		return registry.Record{}, "", fmt.Errorf("VM directory %s is missing; the config cannot be updated", rec.ConfigDir)
	}
	return rec, specPath, nil
}

// runMount attaches a host folder to an existing VM, updating all three
// artifacts that describe it: the VM Spec, the Record, and the Lima runtime.
// The spec and Record are written before the runtime sync, so declining the
// stop/start still leaves those two agreeing with each other — and `avm
// recreate` (or a later mount while stopped) picks the change up from them.
func runMount(ctx context.Context, deps createDeps, vmName, hostPath, name string) error {
	rec, specPath, err := loadVMSpec(deps, vmName)
	if err != nil {
		return err
	}

	abs, err := expandTilde(hostPath)
	if err != nil {
		return err
	}
	if abs, err = filepath.Abs(abs); err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	if err := checkNotVMDir(abs, rec.ConfigDir); err != nil {
		return err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("mount source not found: %s", abs)
	}
	if !fi.IsDir() {
		return fmt.Errorf("mount source is not a directory: %s", abs)
	}

	guestName := name
	if guestName == "" {
		guestName = filepath.Base(abs)
	}
	if err := config.ValidateMountName(guestName); err != nil {
		return err
	}
	guestPath := path.Join(rec.Home, guestName)

	for _, m := range rec.Mounts {
		if m.HostPath == abs {
			fmt.Printf("Already mounted: %s → %s\n", abs, m.GuestPath)
			return nil
		}
	}
	for _, m := range rec.Mounts {
		if m.GuestPath == guestPath {
			return fmt.Errorf("%s and %s both map to %s. Retry with --name <name>.", abs, m.HostPath, guestPath)
		}
	}

	// The spec keeps the ~/ form when the path is under the host home, so the
	// file stays readable and portable between machines.
	if err := specedit.AddMount(specPath, config.MountSpec{Path: tildeForm(abs), Name: name}); err != nil {
		return err
	}
	rec.Mounts = append(rec.Mounts, config.Mount{HostPath: abs, GuestPath: guestPath})
	if err := deps.store.Write(rec); err != nil {
		return fmt.Errorf("agent-vm.yaml was updated but the record could not be saved: %w", err)
	}
	fmt.Printf("Mounted: %s → %s\n", abs, guestPath)
	// Re-running `avm mount` after this point is a no-op (the Record already has
	// the entry), so the error has to say where the change did land.
	if err := syncMounts(ctx, deps, rec, "attach"); err != nil {
		return fmt.Errorf("agent-vm.yaml and the record are updated, but syncing to the VM failed: %w", err)
	}
	return nil
}

// runUnmount detaches a folder named either by its host path or by its guest
// directory name.
func runUnmount(ctx context.Context, deps createDeps, vmName, target string) error {
	rec, specPath, err := loadVMSpec(deps, vmName)
	if err != nil {
		return err
	}

	// One pass, matching either the host path or the guest directory name.
	abs, err := expandTilde(target)
	if err != nil {
		return err
	}
	if abs, err = filepath.Abs(abs); err == nil {
		abs = filepath.Clean(abs)
	}
	idx := -1
	for i, m := range rec.Mounts {
		if m.HostPath == abs || path.Base(m.GuestPath) == target {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("%q is not mounted in %q", target, vmName)
	}
	removed := rec.Mounts[idx]

	removedTilde, err := specedit.RemoveMount(specPath, tildeForm(removed.HostPath))
	if err != nil {
		return err
	}
	// A spec may spell the same folder absolutely; drop that form too.
	removedAbs, err := specedit.RemoveMount(specPath, removed.HostPath)
	if err != nil {
		return err
	}
	if !removedTilde && !removedAbs {
		return fmt.Errorf("%s is tracked in the record but not found in agent-vm.yaml; edit it by hand", removed.HostPath)
	}
	rec.Mounts = append(rec.Mounts[:idx], rec.Mounts[idx+1:]...)
	if err := deps.store.Write(rec); err != nil {
		return err
	}
	fmt.Printf("Unmounted: %s\n", removed.HostPath)
	// As with runMount: the entry is gone from both artifacts, so re-running the
	// command cannot retry the sync.
	if err := syncMounts(ctx, deps, rec, "detach"); err != nil {
		return fmt.Errorf("agent-vm.yaml and the record are updated, but syncing to the VM failed: %w", err)
	}
	return nil
}

// tildeForm rewrites a path under the host home into its ~/ form, so the spec
// stays portable; anything else is returned unchanged.
func tildeForm(abs string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return abs
	}
	rel, err := filepath.Rel(home, abs)
	if err != nil || rel == ".." || filepath.IsAbs(rel) ||
		len(rel) > 1 && rel[0] == '.' && rel[1] == '.' {
		return abs
	}
	if rel == "." {
		return abs
	}
	return "~/" + filepath.ToSlash(rel)
}

func newMountCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "mount <vm-name> [path]",
		Short: "Mount a project folder into a VM",
		// The VM name is explicit: this runs from the project folder, which is not
		// the VM directory, so there is no spec in the cwd to infer it from.
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := cwd()
			if len(args) == 2 {
				target = args[1]
			}
			// Records are stored under the normalized name, so the argument gets
			// the same treatment `avm shell`/`start`/`stop` give it.
			vm, err := vmname.Normalize(args[0])
			if err != nil {
				return err
			}
			root, err := registry.DefaultRoot()
			if err != nil {
				return err
			}
			deps := createDeps{lima: newLimaClient(cmd), store: registry.NewStore(root)}
			return runMount(cmd.Context(), deps, vm, target, name)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "guest directory name (defaults to the folder's basename)")
	return cmd
}

func newUnmountCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unmount <vm-name> <path|name>",
		Short: "Detach a project folder from a VM",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			vm, err := vmname.Normalize(args[0])
			if err != nil {
				return err
			}
			root, err := registry.DefaultRoot()
			if err != nil {
				return err
			}
			deps := createDeps{lima: newLimaClient(cmd), store: registry.NewStore(root)}
			return runUnmount(cmd.Context(), deps, vm, args[1])
		},
	}
}
