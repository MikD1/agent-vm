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
		`"${VM_WORKSPACE}"/'claude-settings.json'`,
		`"${VM_SECRETS}"/'codex-auth.json'`,
		`if [ ! -d '/home/u/.claude' ]; then install -d -m 0755 -o "${VM_USER}" -g "${VM_USER}" '/home/u/.claude'; else mkdir -p '/home/u/.claude'; fi`,
		`cp -R "${VM_WORKSPACE}"/'claude-settings.json' '/home/u/.claude/settings.json'`,
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
		"if [ ! -d " + wantDir + " ]; then install -d -m 0755 -o \"${VM_USER}\" -g \"${VM_USER}\" " + wantDir + "; else mkdir -p " + wantDir + "; fi",
		"cp -R \"${VM_WORKSPACE}\"/'settings.json' " + wantDst,
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

// TestRenderFileCopiesQuotesSource guards against shell injection through
// f.Rel, the relative filename under the project workspace or host secrets
// store. Only root (${VM_WORKSPACE} / ${VM_SECRETS}) must stay double-quoted
// so it expands as a shell variable; f.Rel itself must be single-quoted
// (shellQuote), just like the destination side already is. Before this fix,
// src was built with Go's %q on the whole "root+/+Rel" string, which does not
// escape $ or backticks, so a Rel like "$(rm -rf ~)" would have its command
// substitution executed by bash instead of being treated as a literal (if
// unusual) filename.
func TestRenderFileCopiesQuotesSource(t *testing.T) {
	evil := "$(rm -rf ~)/it's-mine"
	got := string(renderFileCopies([]config.FileCopy{
		{Root: config.RootWorkspace, Rel: evil, To: "/home/u/dst", Mode: "0644"},
		{Root: config.RootSecrets, Rel: evil, To: "/home/u/dst2", Mode: "0644"},
	}))
	if strings.Contains(got, `"${VM_WORKSPACE}/`+evil) || strings.Contains(got, `"${VM_SECRETS}/`+evil) {
		t.Errorf("source Rel was concatenated into the double-quoted root, not single-quoted:\n%s", got)
	}
	wantWorkspaceSrc := `"${VM_WORKSPACE}"` + "/" + shellQuote(evil)
	wantSecretsSrc := `"${VM_SECRETS}"` + "/" + shellQuote(evil)
	for _, want := range []string{wantWorkspaceSrc, wantSecretsSrc} {
		if !strings.Contains(got, want) {
			t.Errorf("renderFileCopies() missing %q:\n%s", want, got)
		}
	}
	// The raw $(...) substitution must never appear unescaped/unquoted inside a
	// double-quoted segment, where bash would execute it as a command.
	if strings.Contains(got, `"${VM_WORKSPACE}/$(rm -rf ~)`) || strings.Contains(got, `"${VM_SECRETS}/$(rm -rf ~)`) {
		t.Errorf("evil source leaked into a double-quoted context:\n%s", got)
	}
}

// TestRenderFileCopiesLeavesExistingDirUntouched proves the generated script
// checks whether the destination's parent directory already exists before
// deciding whether to chown/chmod it. A files entry targeting e.g. /etc or an
// existing ~/.ssh must not reassign that pre-existing directory's ownership to
// the VM user or loosen its mode — only a directory this script actually
// creates gets 0755 + VM-user ownership. This can't be exercised by actually
// running the generated bash from a unit test, so the assertion is on the
// conditional structure itself: an unconditional `install -d` must not appear.
func TestRenderFileCopiesLeavesExistingDirUntouched(t *testing.T) {
	got := string(renderFileCopies([]config.FileCopy{
		{Root: config.RootWorkspace, Rel: "app.conf", To: "/etc/app.conf", Mode: "0644"},
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
