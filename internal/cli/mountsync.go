package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/MikD1/agent-vm/internal/registry"
)

// syncMounts converges the Lima instance onto the Record's mount list. Lima
// attaches virtiofs devices at boot, so there is no incremental "add one mount"
// path: the whole list is rewritten from the Record. One primitive therefore
// serves mount, unmount, and a hand-edited config alike.
//
// `limactl edit` also refuses to touch a running instance, so the config can
// only be rewritten while the VM is stopped. A stopped VM is edited in place; a
// running one goes prompt → stop → edit → start, and declining leaves Lima
// untouched (an edit would only fail, and there is nothing a live guest could
// pick up from it). If the edit fails mid-sequence the VM is left stopped, which
// the caller's error reports.
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

	// `limactl edit` refuses to run against a running instance, so the prompt
	// comes before any Lima call: there is nothing useful to do on decline, and
	// attempting the edit anyway would just fail.
	fmt.Printf("The VM must stop to %s it. Stop and restart it now? [y/N] ", verb)
	var reply string
	fmt.Scanln(&reply)
	if reply != "y" && reply != "Y" {
		fmt.Printf("Declined. The change is recorded in agent-vm.yaml and the record but is not applied to the VM — a plain `avm restart` will not pick it up. Stop the VM (`avm stop %s`) and run the same command again, or `avm recreate %s` to rebuild it with the current mounts.\n", rec.Name, rec.Name)
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
