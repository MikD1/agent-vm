package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/registry"
	"github.com/MikD1/agent-vm/internal/vmname"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newMountCmd() *cobra.Command {
	var guestPath string
	var writable bool

	cmd := &cobra.Command{
		Use:   "mount <host-path> [<vm>]",
		Short: "Add a host directory mount to an existing VM (requires VM restart)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			limaClient := newLimaClient(cmd)

			hostPath, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}
			if _, err := os.Stat(hostPath); err != nil {
				return fmt.Errorf("host path %q: %w", hostPath, err)
			}

			name := ""
			if len(args) == 2 {
				name = args[1]
			} else {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				name, err = vmname.Normalize(filepath.Base(cwd))
				if err != nil {
					return fmt.Errorf("cannot derive VM name from cwd; pass VM name explicitly: %w", err)
				}
			}

			root, err := registry.DefaultRoot()
			if err != nil {
				return err
			}
			store := registry.NewStore(root)
			rec, err := store.Read(name)
			if err != nil {
				return err
			}

			gp := guestPath
			if gp == "" {
				guestHome := filepath.Dir(rec.Workspace.GuestPath)
				gp = guestHome + "/" + filepath.Base(hostPath)
			}

			vmDir, err := limaClient.VMDir(ctx, name)
			if err != nil {
				return err
			}
			configPath := filepath.Join(vmDir, "lima.yaml")

			data, err := os.ReadFile(configPath)
			if err != nil {
				return fmt.Errorf("read Lima config: %w", err)
			}
			var doc map[string]any
			if err := yaml.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("parse Lima config: %w", err)
			}

			mounts, _ := doc["mounts"].([]any)
			for _, m := range mounts {
				mm, ok := m.(map[string]any)
				if !ok {
					continue
				}
				if mm["location"] == hostPath {
					return fmt.Errorf("%q is already mounted in VM %q", hostPath, name)
				}
				if mm["mountPoint"] == gp {
					return fmt.Errorf("guest path %q is already in use in VM %q", gp, name)
				}
			}

			mounts = append(mounts, map[string]any{
				"location":   hostPath,
				"mountPoint": gp,
				"writable":   writable,
			})
			doc["mounts"] = mounts

			updated, err := yaml.Marshal(doc)
			if err != nil {
				return err
			}

			fmt.Printf("Stopping VM %q...\n", name)
			if err := limaClient.Stop(ctx, name); err != nil {
				return fmt.Errorf("stop VM: %w", err)
			}

			if err := os.WriteFile(configPath, updated, 0o644); err != nil {
				return fmt.Errorf("write Lima config: %w", err)
			}

			fmt.Printf("Starting VM %q...\n", name)
			if err := limaClient.Start(ctx, name); err != nil {
				return fmt.Errorf("start VM: %w", err)
			}

			rec.ExtraMounts = append(rec.ExtraMounts, config.ExtraMount{
				HostPath:  hostPath,
				GuestPath: gp,
				Writable:  writable,
			})
			if err := store.Write(rec); err != nil {
				return fmt.Errorf("update record: %w", err)
			}

			fmt.Printf("Mounted %q at %q in VM %q\n", hostPath, gp, name)
			return nil
		},
	}
	cmd.Flags().StringVar(&guestPath, "guest-path", "", "mount point inside the VM (default: ~/basename(host-path))")
	cmd.Flags().BoolVar(&writable, "writable", false, "make the mount writable")
	return cmd
}
