package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/MikD1/agent-vm/internal/registry"
)

// syncMounts converges the Lima instance onto the Record's mount list. Lima
// attaches virtiofs devices at boot, so both mount and unmount rewrite the
// whole list from the Record.
//
// After checking the VM's state, edit a stopped VM in place or ask for
// confirmation before stopping, editing, and starting a running VM. On
// decline, the state check is the only Lima operation.
//
// verb is "attach" or "detach" — the only thing that differs between the two
// commands is what the prompt says.
func syncMounts(ctx context.Context, deps createDeps, rec registry.Record, verb string) error {
	instances, err := deps.lima.Instances(ctx)
	if err != nil {
		return err
	}
	state := ""
	for _, inst := range instances {
		if inst.Name == rec.Name {
			state = inst.State
		}
	}
	if state == "" {
		// No VM behind this Record: the config and Record are already updated, and
		// `avm recreate` will build it with the current list.
		return nil
	}

	// The value is a JSON array, which is valid YAML flow syntax — so no manual
	// escaping is needed for paths containing spaces or quotes.
	encoded, err := json.Marshal(limaMounts(rec.ConfigDir, rec.Mounts))
	if err != nil {
		return err
	}
	expr := ".mounts = " + string(encoded)

	// A stopped VM can be edited straight away; the new list is attached the next
	// time it boots.
	if state != "running" {
		return deps.lima.EditMounts(ctx, rec.Name, expr)
	}

	fmt.Printf("The VM must stop to %s it. Stop and restart it now? [y/N] ", verb)
	var reply string
	fmt.Scanln(&reply)
	if reply != "y" && reply != "Y" {
		fmt.Println("Declined. The mount change was saved to agent-vm.yaml and the record.")
		fmt.Println("An ordinary `avm restart` uses the existing Lima configuration. Repeating this command skips synchronization.")
		fmt.Printf("`avm recreate %s` applies the saved mounts by rebuilding the VM. Rebuilding deletes data stored only inside the VM.\n", rec.Name)
		return nil
	}
	fmt.Printf("==> Stopping VM: %s\n", rec.Name)
	if err := deps.lima.Stop(ctx, rec.Name); err != nil {
		return err
	}
	if err := deps.lima.EditMounts(ctx, rec.Name, expr); err != nil {
		return err
	}
	fmt.Printf("==> Starting VM: %s\n", rec.Name)
	return deps.lima.Start(ctx, rec.Name)
}
