package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/registry"
)

type sshConfigRunner struct {
	okRunner
	path         string
	lookupErr    error
	deleteErr    error
	provisionErr error
}

func (r *sshConfigRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	out, stderr, err := r.okRunner.Run(ctx, stdin, args...)
	if slices.Equal(args, []string{"list", "--format", "{{.SSHConfigFile}}", "work"}) {
		return []byte(r.path + "\n"), nil, r.lookupErr
	}
	switch args[0] {
	case "delete":
		return out, stderr, r.deleteErr
	case "shell":
		return out, stderr, r.provisionErr
	}
	return out, stderr, err
}

func TestCreateLooksUpSSHConfigAfterRestart(t *testing.T) {
	runner := &sshConfigRunner{path: filepath.Join(t.TempDir(), "Lima VMs", "work", "ssh.config")}
	ssh := testSSHConfig(t)
	deps := createDeps{lima: lima.New(runner), store: registry.NewStore(t.TempDir()), ssh: ssh}
	r := config.Resolved{Name: "work", User: "me", ConfigDir: t.TempDir(), Home: "/home/me"}
	if err := runCreate(t.Context(), deps, r, r.Home, nowFixed(), false); err != nil {
		t.Fatal(err)
	}
	if got := runner.ops[len(runner.ops)-2:]; !slices.Equal(got, []string{"restart", "list"}) {
		t.Errorf("last Lima operations = %v, want [restart list]", got)
	}
	if got, want := readSSHConfig(t, ssh.path), fmt.Sprintf("Include %q # avm: work\n", runner.path); got != want {
		t.Errorf("config = %q, want %q", got, want)
	}
	assertFileMode(t, filepath.Dir(ssh.path), 0o700)
	assertFileMode(t, ssh.path, 0o600)
}

func TestRecreateUpdatesSSHIncludeAndDeleteRemovesIt(t *testing.T) {
	ssh := testSSHConfig(t)
	old := "  Include \"/old-home/work/ssh.config\"  # avm: work\n"
	other := "Include \"/lima/work-extra/ssh.config\" # avm: work-extra\n"
	manual := "# preserve these bytes\r\nInclude /old-home/work/ssh.config\r\nHost *\r\n  User me"
	writeSSHConfig(t, ssh.path, old+other+manual)
	runner := &sshConfigRunner{path: "/new lima home/work/ssh.config"}
	store := registry.NewStore(t.TempDir())
	r := config.Resolved{Name: "work", User: "me", Home: "/home/me"}
	if err := store.Write(registry.FromResolved(r, nowFixed())); err != nil {
		t.Fatal(err)
	}
	deps := createDeps{lima: lima.New(runner), store: store, ssh: ssh}
	for range 2 {
		if err := runRecreate(t.Context(), deps, "work", false); err != nil {
			t.Fatal(err)
		}
	}
	want := "Include \"/new lima home/work/ssh.config\" # avm: work\n" + other + manual
	if got := readSSHConfig(t, ssh.path); got != want {
		t.Errorf("config = %q, want %q", got, want)
	}
	if err := runDelete(t.Context(), deps.lima, store, ssh, "work"); err != nil {
		t.Fatal(err)
	}
	if got := readSSHConfig(t, ssh.path); got != other+manual {
		t.Errorf("config after delete = %q, want %q", got, other+manual)
	}
}

func TestFailedDeleteKeepsSSHInclude(t *testing.T) {
	ssh := testSSHConfig(t)
	entry := "Include \"/lima/work/ssh.config\" # avm: work\n"
	writeSSHConfig(t, ssh.path, entry)
	runner := &sshConfigRunner{deleteErr: errors.New("delete failed")}
	if err := runDelete(t.Context(), lima.New(runner), registry.NewStore(t.TempDir()), ssh, "work"); err == nil {
		t.Fatal("want a Lima deletion error")
	}
	if got := readSSHConfig(t, ssh.path); got != entry {
		t.Errorf("config = %q, want %q", got, entry)
	}
}

func TestRecreateRollbackRemovesSSHInclude(t *testing.T) {
	ssh := testSSHConfig(t)
	writeSSHConfig(t, ssh.path, "Include \"/lima/work/ssh.config\" # avm: work\n")
	runner := &sshConfigRunner{provisionErr: errors.New("provisioning failed")}
	store := registry.NewStore(t.TempDir())
	r := config.Resolved{Name: "work", User: "me", Home: "/home/me"}
	if err := store.Write(registry.FromResolved(r, nowFixed())); err != nil {
		t.Fatal(err)
	}
	deps := createDeps{lima: lima.New(runner), store: store, ssh: ssh}
	if err := runRecreate(t.Context(), deps, "work", false); err == nil {
		t.Fatal("want a provisioning error")
	}
	if got := readSSHConfig(t, ssh.path); got != "" {
		t.Errorf("config after rollback = %q, want an empty file", got)
	}
}

func TestCreateWarnsWhenSSHLookupFails(t *testing.T) {
	ssh := testSSHConfig(t)
	var warnings bytes.Buffer
	ssh.warnings = &warnings
	runner := &sshConfigRunner{lookupErr: errors.New("lookup failed")}
	deps := createDeps{lima: lima.New(runner), store: registry.NewStore(t.TempDir()), ssh: ssh}
	r := config.Resolved{Name: "work", User: "me", ConfigDir: t.TempDir(), Home: "/home/me"}
	if err := runCreate(t.Context(), deps, r, r.Home, nowFixed(), false); err != nil {
		t.Fatalf("SSH lookup failure must leave creation successful: %v", err)
	}
	if got := warnings.String(); !strings.Contains(got, "Warning:") || !strings.Contains(got, "lookup failed") {
		t.Errorf("warning = %q, want the Lima lookup failure", got)
	}
	if _, err := os.Stat(ssh.path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("SSH config stat = %v, want a missing config", err)
	}
}

func TestSSHConfigEditPreservesSymlinkAndMode(t *testing.T) {
	target := filepath.Join(t.TempDir(), "dotfiles-ssh-config")
	writeSSHConfig(t, target, "Host other\n  Port 2200\n")
	const mode = 0o640 | os.ModeSetgid
	if err := os.Chmod(target, mode); err != nil {
		t.Fatal(err)
	}
	assertFileMode(t, target, mode)
	ssh := testSSHConfig(t)
	if err := os.Mkdir(filepath.Dir(ssh.path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, ssh.path); err != nil {
		t.Fatal(err)
	}
	if err := editSSHConfig(ssh.path, "work", "/lima/work/ssh.config"); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(ssh.path); err != nil || got != target {
		t.Fatalf("config link = %q, %v, want %q", got, err, target)
	}
	if got := readSSHConfig(t, target); got != "Include \"/lima/work/ssh.config\" # avm: work\nHost other\n  Port 2200\n" {
		t.Errorf("config target = %q", got)
	}
	assertFileMode(t, target, mode)
}

func TestSSHConfigOpenSSHResolvesLimaAlias(t *testing.T) {
	binary, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("OpenSSH is unavailable")
	}
	ssh := testSSHConfig(t)
	source := filepath.Join(t.TempDir(), "Lima VMs [local]", "work", "ssh.config")
	writeSSHConfig(t, source, "Host lima-work\n  HostName 127.0.0.1\n  Port 60123\n")
	writeSSHConfig(t, ssh.path, "Host *\n  Port 2200\n")
	if err := editSSHConfig(ssh.path, "work", source); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ host, hostname, port string }{
		{"lima-work", "127.0.0.1", "60123"},
		{"other", "other", "2200"},
	} {
		t.Run(tt.host, func(t *testing.T) {
			out, err := exec.CommandContext(t.Context(), binary, "-G", "-F", ssh.path, tt.host).CombinedOutput()
			if err != nil {
				t.Fatalf("ssh -G: %v\n%s", err, out)
			}
			if !strings.Contains(string(out), "\nhostname "+tt.hostname+"\n") ||
				!strings.Contains(string(out), "\nport "+tt.port+"\n") {
				t.Errorf("want hostname %s and port %s in ssh config for %s, got:\n%s", tt.hostname, tt.port, tt.host, out)
			}
		})
	}
}

func TestSSHConfigConcurrentEditsKeepAllIncludes(t *testing.T) {
	ssh := testSSHConfig(t)
	var edits sync.WaitGroup
	for i := range 12 {
		name := fmt.Sprintf("work-%d", i)
		edits.Go(func() {
			if err := editSSHConfig(ssh.path, name, "/lima/"+name+"/ssh.config"); err != nil {
				t.Error(err)
			}
		})
	}
	edits.Wait()
	got := readSSHConfig(t, ssh.path)
	for i := range 12 {
		if entry := fmt.Sprintf("Include \"/lima/work-%d/ssh.config\" # avm: work-%d\n", i, i); !strings.Contains(got, entry) {
			t.Errorf("missing entry %q", entry)
		}
	}
}

func testSSHConfig(t *testing.T) sshConfig {
	t.Helper()
	return sshConfig{path: filepath.Join(t.TempDir(), ".ssh", "config"), warnings: &bytes.Buffer{}}
}

func writeSSHConfig(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readSSHConfig(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky); got != want {
		t.Errorf("%s permissions = %v, want %v", path, got, want)
	}
}
