package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/provision"
	"github.com/MikD1/agent-vm/internal/registry"
	"github.com/spf13/cobra"
)

// recordToResolved rebuilds the Resolved config from a stored Record.
//
// A Record written before modules carried a per-tool version (or hand-edited
// to the old `modules: [name, ...]` form) can have a ModuleSpec with an empty
// Version. config.Resolve normally backfills that to config.DefaultToolVersion
// on the fresh-create path (internal/config/resolve.go); recordToResolved
// bypasses Resolve entirely, so it must apply the same backfill itself —
// otherwise renderMiseConfig would emit a broken `"node" = ""` TOML entry.
func recordToResolved(rec registry.Record) config.Resolved {
	modules := append([]config.ModuleSpec(nil), rec.Modules...)
	for i := range modules {
		if modules[i].Version == "" {
			modules[i].Version = config.DefaultToolVersion
		}
	}
	return config.Resolved{
		Name: rec.Name, User: rec.User,
		Modules: modules, Resources: rec.Resources, Base: rec.Base,
		ConfigDir: rec.ConfigDir, Home: rec.Home,
		Mounts: rec.Mounts, Files: rec.Files, Scripts: rec.Scripts,
	}
}

// resolveForRecreate produces the config to rebuild from. The VM Spec is the
// source and the Record a snapshot of it, so the Spec wins whenever it is
// readable: the documented model is "change agent-vm.yaml, then `avm recreate`",
// and a rebuild that replayed the snapshot would silently ignore the edit that
// prompted it. The Record still supplies the create-time facts a Spec does not
// carry — the guest user, the guest home, and the VM name this rebuild targets
// (a renamed `name:` key must not redirect the rebuild at a different VM).
//
// When the VM directory is not on this host, the Record's own snapshot is used.
// That is what keeps `recreate` working without the VM directory: recovering an
// orphaned Record on a machine that never had the folder, per architecture §5.
// The returned string names the source, for the caller to report.
func resolveForRecreate(rec registry.Record) (config.Resolved, string, error) {
	specPath := filepath.Join(rec.ConfigDir, "agent-vm.yaml")
	if rec.ConfigDir == "" {
		return recordToResolved(rec), "its record", nil
	}
	if _, err := os.Stat(specPath); err != nil {
		return recordToResolved(rec), fmt.Sprintf("its record (%s not found)", specPath), nil
	}
	spec, err := config.Load(specPath)
	if err != nil {
		return config.Resolved{}, "", err
	}
	if err := spec.Validate(); err != nil {
		return config.Resolved{}, "", fmt.Errorf("%s: %w", specPath, err)
	}
	mounts, err := resolveMountInputs(spec.Mounts, rec.ConfigDir)
	if err != nil {
		return config.Resolved{}, "", err
	}
	files, err := resolveFileInputs(spec.Files, rec.ConfigDir)
	if err != nil {
		return config.Resolved{}, "", err
	}
	scripts, err := resolveScriptInputs(spec.Scripts, rec.ConfigDir)
	if err != nil {
		return config.Resolved{}, "", err
	}
	r, err := config.Resolve(config.Flags{}, spec, config.Env{
		ProjectName: rec.Name,
		GuestUser:   rec.User,
		GuestHome:   rec.Home,
		ConfigDir:   rec.ConfigDir,
		Mounts:      mounts,
		Files:       files,
		Scripts:     scripts,
	})
	if err != nil {
		return config.Resolved{}, "", err
	}
	return r, specPath, nil
}

// runRecreate reads the Record, re-resolves the config, deletes any existing VM,
// and rebuilds pristinely. The guest home comes from the Record rather than from
// `limactl info`: the Record holds the home this VM was actually built with.
func runRecreate(ctx context.Context, deps createDeps, name string, verbose bool) error {
	rec, err := deps.store.Read(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no record for %q; nothing to recreate", name)
		}
		return fmt.Errorf("read record for %q: %w", name, err)
	}
	r, source, err := resolveForRecreate(rec)
	if err != nil {
		return err
	}
	fmt.Printf("==> Rebuilding %s from %s\n", name, source)

	limaYAML, err := buildLimaConfig(r, rec.Home)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "avm-"+name+"-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(limaYAML); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	_ = deps.lima.Delete(ctx, name) // pristine: remove any existing VM first
	p := provision.New(deps.lima, verbose)
	if provErr := p.Run(ctx, r, tmp.Name()); provErr != nil {
		rollbackMsg := "VM rolled back"
		if delErr := deps.lima.Delete(ctx, name); delErr != nil {
			rollbackMsg = fmt.Sprintf("VM rollback attempted but may have failed (%v); verify with `limactl list`", delErr)
		}
		return fmt.Errorf("%w\n%s; record kept. Run `avm recreate %s` to retry", provErr, rollbackMsg, name)
	}
	// The Record is rewritten from what was just built, so it keeps describing the
	// VM that exists. CreatedAt is carried over: recreating is not creating.
	next := registry.FromResolved(r, rec.CreatedAt)
	next.InstalledTools = p.InstalledTools()
	if err := deps.store.Write(next); err != nil {
		fmt.Printf("Warning: could not update the record: %v\n", err)
	}
	fmt.Printf("VM recreated: %s\n", name)
	return nil
}

func newRecreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "recreate <name>",
		Short: "Pristine rebuild of a VM from its record (anything living only inside the guest is lost)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			limaClient := newLimaClient(cmd)
			root, err := registry.DefaultRoot()
			if err != nil {
				return err
			}
			store := registry.NewStore(root)
			deps := createDeps{lima: limaClient, store: store}
			verbose, _ := cmd.Flags().GetBool("verbose")
			return runRecreate(ctx, deps, args[0], verbose)
		},
	}
}
