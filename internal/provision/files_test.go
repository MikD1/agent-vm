package provision

import (
	"path"
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
)

func TestRenderFileCopies(t *testing.T) {
	got := string(renderFileCopies([]config.FileCopy{
		{Root: config.RootWorkspace, Rel: "claude-settings.json", To: "/home/u/.claude/settings.json", Mode: "0644"},
		{Root: config.RootSecrets, Rel: "codex-auth.json", To: "/home/u/.codex/auth.json", Mode: "0600"},
		{Root: config.RootWorkspace, Rel: "agents", To: "/home/u/.claude/agents", IsDir: true},
	}))
	for _, want := range []string{
		`"${VM_WORKSPACE}/claude-settings.json"`,
		`"${VM_SECRETS}/codex-auth.json"`,
		`install -d -m 0755 -o "${VM_USER}" -g "${VM_USER}" '/home/u/.claude'`,
		`cp -R "${VM_WORKSPACE}/claude-settings.json" '/home/u/.claude/settings.json'`,
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
		{Root: config.RootWorkspace, Rel: "settings.json", To: evil, Mode: "0644"},
	}))
	if strings.Contains(got, `"`+evil+`"`) {
		t.Errorf("destination was double-quoted (shell-expandable), not single-quoted:\n%s", got)
	}
	wantDst := shellQuote(evil)
	wantDir := shellQuote(path.Dir(evil))
	for _, want := range []string{
		"install -d -m 0755 -o \"${VM_USER}\" -g \"${VM_USER}\" " + wantDir,
		"cp -R \"${VM_WORKSPACE}/settings.json\" " + wantDst,
		"chown -R \"${VM_USER}:${VM_USER}\" " + wantDst,
		"chmod 0644 " + wantDst,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderFileCopies() missing %q:\n%s", want, got)
		}
	}
	// The raw $(...) substitution must never appear unescaped/unquoted such that
	// bash would treat it as a command substitution boundary outside quotes.
	if strings.Contains(got, `$(rm -rf ~)/it's-mine"`) {
		t.Errorf("evil destination leaked into a double-quoted context:\n%s", got)
	}
}
