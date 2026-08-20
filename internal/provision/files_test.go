package provision

import (
	"path"
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
)

func TestRenderFileCopies(t *testing.T) {
	got := string(renderFileCopies([]config.FileCopy{
		{Rel: "claude-settings.json", To: "/home/u/.claude/settings.json", Mode: "0644"},
		{Rel: "codex-auth.json", To: "/home/u/.codex/auth.json", Mode: "0600"},
		{Rel: "agents", To: "/home/u/.claude/agents", IsDir: true},
	}))
	for _, want := range []string{
		`"${VM_CONFIG}"/'claude-settings.json'`,
		`"${VM_CONFIG}"/'codex-auth.json'`,
		`if [ ! -d '/home/u/.claude' ]; then install -d -m 0755 -o "${VM_USER}" -g "${VM_USER}" '/home/u/.claude'; else mkdir -p '/home/u/.claude'; fi`,
		`cp -R "${VM_CONFIG}"/'claude-settings.json' '/home/u/.claude/settings.json'`,
		`chown -R "${VM_USER}:${VM_USER}" '/home/u/.claude/settings.json'`,
		`chmod 0600 '/home/u/.codex/auth.json'`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderFileCopies() missing %q:\n%s", want, got)
		}
	}
	// A directory keeps the modes cp -R preserves, so no chmod is emitted for it.
	if strings.Contains(got, `chmod  '/home/u/.claude/agents'`) {
		t.Errorf("directory entry got a chmod:\n%s", got)
	}
}

func TestRenderFileCopiesEmpty(t *testing.T) {
	if got := renderFileCopies(nil); len(got) != 0 {
		t.Errorf("renderFileCopies(nil) = %q, want empty", got)
	}
}

// TestRenderFileCopiesQuotesDestination guards against shell injection through a
// malicious `to:` destination. The destination lands inside a bash
// double-quoted argument (cp/chown/chmod/install -d), and Go's %q does not
// escape $ or backticks — so a To like "~/x/$(rm -rf ~)/y" would previously
// have its $(...) substitution executed by bash instead of being treated as a
// literal (if odd) path component. renderFileCopies must single-quote (via
// shellQuote) the destination and its parent directory so it can never be
// interpreted as anything but a literal string.
func TestRenderFileCopiesQuotesDestination(t *testing.T) {
	evil := "/home/u/$(rm -rf ~)/it's-mine"
	got := string(renderFileCopies([]config.FileCopy{
		{Rel: "settings.json", To: evil, Mode: "0644"},
	}))
	if strings.Contains(got, `"`+evil+`"`) {
		t.Errorf("destination was double-quoted (shell-expandable), not single-quoted:\n%s", got)
	}
	wantDst := shellQuote(evil)
	wantDir := shellQuote(path.Dir(evil))
	for _, want := range []string{
		"if [ ! -d " + wantDir + " ]; then install -d -m 0755 -o \"${VM_USER}\" -g \"${VM_USER}\" " + wantDir + "; else mkdir -p " + wantDir + "; fi",
		"cp -R \"${VM_CONFIG}\"/'settings.json' " + wantDst,
		"chown -R \"${VM_USER}:${VM_USER}\" " + wantDst,
		"chmod 0644 " + wantDst,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderFileCopies() missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `$(rm -rf ~)/it's-mine"`) {
		t.Errorf("evil destination leaked into a double-quoted context:\n%s", got)
	}
}

// TestRenderFileCopiesQuotesSource guards against shell injection through f.Rel,
// the path relative to the VM directory. Only the root (${VM_CONFIG}) must stay
// double-quoted so it expands as a shell variable; f.Rel itself must be
// single-quoted (shellQuote), just like the destination side already is.
func TestRenderFileCopiesQuotesSource(t *testing.T) {
	evil := "$(rm -rf ~)/it's-mine"
	got := string(renderFileCopies([]config.FileCopy{
		{Rel: evil, To: "/home/u/dst", Mode: "0644"},
	}))
	if strings.Contains(got, `"${VM_CONFIG}/`+evil) {
		t.Errorf("source Rel was concatenated into the double-quoted root, not single-quoted:\n%s", got)
	}
	want := `"${VM_CONFIG}"` + "/" + shellQuote(evil)
	if !strings.Contains(got, want) {
		t.Errorf("renderFileCopies() missing %q:\n%s", want, got)
	}
	if strings.Contains(got, `"${VM_CONFIG}/$(rm -rf ~)`) {
		t.Errorf("evil source leaked into a double-quoted context:\n%s", got)
	}
}

// TestRenderFileCopiesLeavesExistingDirUntouched proves the generated script
// checks whether the destination's parent directory already exists before
// deciding whether to chown/chmod it. A files entry targeting e.g. /etc or an
// existing ~/.ssh must not reassign that pre-existing directory's ownership to
// the VM user or loosen its mode — only a directory this script actually
// creates gets 0755 + VM-user ownership.
func TestRenderFileCopiesLeavesExistingDirUntouched(t *testing.T) {
	got := string(renderFileCopies([]config.FileCopy{
		{Rel: "app.conf", To: "/etc/app.conf", Mode: "0644"},
	}))
	dir := shellQuote("/etc")
	if strings.Contains(got, "\ninstall -d -m 0755") {
		t.Errorf("install -d must be conditional on the directory not already existing, found an unconditional one:\n%s", got)
	}
	want := "if [ ! -d " + dir + " ]; then install -d -m 0755 -o \"${VM_USER}\" -g \"${VM_USER}\" " + dir + "; else mkdir -p " + dir + "; fi"
	if !strings.Contains(got, want) {
		t.Errorf("renderFileCopies() missing the existence-checked install -d:\n%s", got)
	}
}
