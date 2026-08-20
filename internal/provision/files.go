package provision

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"github.com/MikD1/agent-vm/internal/config"
)

// renderFileCopies is the phase 4 script: copy each declared file out of the VM
// directory and into its destination, owned by the VM user. Sources are
// addressed through the env contract (VM_CONFIG) rather than through host paths,
// so the same entries work on any machine that has the VM directory.
//
// Returns an empty slice when there is nothing to copy, so the caller can skip
// the phase entirely.
func renderFileCopies(files []config.FileCopy) []byte {
	if len(files) == 0 {
		return nil
	}
	var b bytes.Buffer
	for _, f := range files {
		// The root must stay double-quoted so ${VM_CONFIG} expands; f.Rel is a
		// relative path out of the VM directory and is not restricted to
		// shell-safe characters, so it is single-quoted (shellQuote) separately.
		// Go's %q on the combined string would not escape $/backticks in f.Rel,
		// letting a name like "$(...)" execute as a command — adjacent double- and
		// single-quoted bash strings concatenate into one word, so this still
		// resolves to "${VM_CONFIG}/relative/path" with no double slash.
		src := `"${VM_CONFIG}"/` + shellQuote(f.Rel)
		// dst comes from the guest destination in the Spec, which is not restricted
		// to shell-safe characters. Single-quote it (shellQuote), unlike src, which
		// must stay double-quoted so ${VM_CONFIG} expands.
		dst := shellQuote(f.To)
		dir := shellQuote(path.Dir(f.To))
		// Only chown/chmod a directory this script actually creates. A pre-existing
		// parent (e.g. /etc, or an already-0700 ~/.ssh) must keep its own ownership
		// and mode; `mkdir -p` on an existing directory is a no-op that leaves it
		// untouched.
		fmt.Fprintf(&b, "if [ ! -d %s ]; then install -d -m 0755 -o \"${VM_USER}\" -g \"${VM_USER}\" %s; else mkdir -p %s; fi\n", dir, dir, dir)
		fmt.Fprintf(&b, "cp -R %s %s\n", src, dst)
		fmt.Fprintf(&b, "chown -R \"${VM_USER}:${VM_USER}\" %s\n", dst)
		if f.Mode != "" {
			fmt.Fprintf(&b, "chmod %s %s\n", f.Mode, dst)
		}
	}
	return b.Bytes()
}

// shellQuote wraps s in single quotes and escapes any embedded single quotes,
// producing a bash-safe argument regardless of the string's content.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
