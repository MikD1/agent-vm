package provision

import (
	"embed"
	"fmt"
)

//go:embed scripts/*.sh
var guestScripts embed.FS

// miseVersion pins the mise release installed into every VM. It replaces the
// three per-tool pins the old modules carried (a Node major, a .NET channel, and
// "latest stable" for Go): mise now decides tool versions, from the Spec.
const miseVersion = "v2026.8.4"

// guestScript returns an embedded platform script by base name (no .sh suffix).
func guestScript(name string) ([]byte, error) {
	b, err := guestScripts.ReadFile("scripts/" + name + ".sh")
	if err != nil {
		return nil, fmt.Errorf("embedded script %q not found", name)
	}
	return b, nil
}
