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
//
// Keep this at v2026.8.9 or newer. The `claude` module resolves to the aqua
// package anthropics/claude-code, whose recent versions are served by a
// version_override that switches the package type to github_release; mise
// ignored an explicitly overridden type until v2026.8.5, kept the parent http
// package's empty url and failed phase 3 with "builder error: relative URL
// without a base". v2026.8.9 then taught the aqua backend to prefer the glibc
// asset of that same package over the musl one.
const miseVersion = "v2026.8.10"

// guestScript returns an embedded platform script by base name (no .sh suffix).
func guestScript(name string) ([]byte, error) {
	b, err := guestScripts.ReadFile("scripts/" + name + ".sh")
	if err != nil {
		return nil, fmt.Errorf("embedded script %q not found", name)
	}
	return b, nil
}
