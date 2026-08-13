package provision

import (
	"bytes"
	"fmt"
	"path"

	"github.com/MikD1/agent-vm/internal/config"
)

// renderFileCopies is the phase 5 script: copy each declared file out of a mount
// and into its destination, owned by the VM user. Sources are addressed through
// the env contract (VM_WORKSPACE, VM_SECRETS) rather than through host paths, so
// the same entries work on any machine that has the project checked out.
//
// Returns an empty slice when there is nothing to copy, so the caller can skip
// the phase entirely.
func renderFileCopies(files []config.FileCopy) []byte {
	if len(files) == 0 {
		return nil
	}
	var b bytes.Buffer
	for _, f := range files {
		root := "${VM_WORKSPACE}"
		if f.Root == config.RootSecrets {
			root = "${VM_SECRETS}"
		}
		src := fmt.Sprintf("%q", root+"/"+f.Rel)
		// dst comes from the guest destination in the Spec, which is not restricted
		// to shell-safe characters. Single-quote it (shellQuote), unlike src, which
		// must stay double-quoted so ${VM_WORKSPACE}/${VM_SECRETS} expand.
		dst := shellQuote(f.To)
		fmt.Fprintf(&b, "install -d -m 0755 -o \"${VM_USER}\" -g \"${VM_USER}\" %s\n", shellQuote(path.Dir(f.To)))
		fmt.Fprintf(&b, "cp -R %s %s\n", src, dst)
		fmt.Fprintf(&b, "chown -R \"${VM_USER}:${VM_USER}\" %s\n", dst)
		if f.Mode != "" {
			fmt.Fprintf(&b, "chmod %s %s\n", f.Mode, dst)
		}
	}
	return b.Bytes()
}
