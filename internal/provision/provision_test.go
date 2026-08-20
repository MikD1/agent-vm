package provision

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/lima"
)

type recorder struct {
	args  [][]string
	stdin []string
}

func (r *recorder) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	r.args = append(r.args, args)
	r.stdin = append(r.stdin, string(stdin))
	return []byte("{}"), nil, nil
}

func ops(r *recorder) []string {
	var out []string
	for _, a := range r.args {
		out = append(out, a[0])
	}
	return out
}

func baseResolved() config.Resolved {
	return config.Resolved{
		Name:      "work",
		Modules:   []config.ModuleSpec{{Name: "node", Version: "lts"}},
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		User:      "me",
		ConfigDir: "/Users/me/vms/work",
		Home:      "/home/me.linux",
	}
}

func TestPlanPhaseOrder(t *testing.T) {
	rec := &recorder{}
	p := New(lima.New(rec), false)
	if err := p.Run(context.Background(), baseResolved(), "/tmp/cfg.yaml"); err != nil {
		t.Fatal(err)
	}
	// create, start, 4 platform provisions (system, base, docker, mise),
	// 1 mise install, 1 mise ls read-back, then the unconditional restart.
	want := []string{"create", "start", "shell", "shell", "shell", "shell", "shell", "shell", "restart"}
	if got := ops(rec); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ops = %v, want %v", got, want)
	}
}

func TestPlanRestartsWithoutDocker(t *testing.T) {
	// Docker is platform, not a module: the restart no longer depends on modules.
	rec := &recorder{}
	r := baseResolved()
	r.Modules = nil
	p := New(lima.New(rec), false)
	if err := p.Run(context.Background(), r, "/tmp/cfg.yaml"); err != nil {
		t.Fatal(err)
	}
	want := []string{"create", "start", "shell", "shell", "shell", "shell", "shell", "shell", "restart"}
	if got := ops(rec); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ops = %v, want %v", got, want)
	}
}

func TestPlanFilesPhase(t *testing.T) {
	rec := &recorder{}
	r := baseResolved()
	r.Files = []config.FileCopy{
		{Rel: "settings.json", To: "/home/me.linux/.claude/settings.json", Mode: "0644"},
	}
	p := New(lima.New(rec), false)
	if err := p.Run(context.Background(), r, "/tmp/cfg.yaml"); err != nil {
		t.Fatal(err)
	}
	var sawCopy bool
	for _, in := range rec.stdin {
		if strings.Contains(in, `cp -R "${VM_CONFIG}"/'settings.json'`) {
			sawCopy = true
		}
	}
	if !sawCopy {
		t.Errorf("no files phase ran; stdin was %#v", rec.stdin)
	}
}

func TestPlanSkipsEmptyFilesPhase(t *testing.T) {
	rec := &recorder{}
	p := New(lima.New(rec), false)
	if err := p.Run(context.Background(), baseResolved(), "/tmp/cfg.yaml"); err != nil {
		t.Fatal(err)
	}
	// Same op count as the base case: no extra shell for an empty files list.
	want := []string{"create", "start", "shell", "shell", "shell", "shell", "shell", "shell", "restart"}
	if got := ops(rec); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ops = %v, want %v", got, want)
	}
}

func TestPlanScriptsPhase(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.sh")
	second := filepath.Join(dir, "second.sh")
	if err := os.WriteFile(first, []byte("#!/usr/bin/env bash\necho first\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("#!/usr/bin/env bash\necho second\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	r := baseResolved()
	r.Scripts = []string{first, second}
	p := New(lima.New(rec), false)
	if err := p.Run(context.Background(), r, "/tmp/cfg.yaml"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(rec.stdin, "\n---\n")
	if !strings.Contains(joined, "echo first") || !strings.Contains(joined, "echo second") {
		t.Errorf("scripts did not run; stdin was %#v", rec.stdin)
	}
	if strings.Index(joined, "echo first") > strings.Index(joined, "echo second") {
		t.Error("scripts ran out of spec order")
	}
}

func TestPlanScriptsPhaseMissingFile(t *testing.T) {
	rec := &recorder{}
	r := baseResolved()
	r.Scripts = []string{filepath.Join(t.TempDir(), "gone.sh")}
	p := New(lima.New(rec), false)
	err := p.Run(context.Background(), r, "/tmp/cfg.yaml")
	if err == nil {
		t.Fatal("missing script = nil error, want an error")
	}
	// User scripts are phase 5: config files are 4, the restart is 6.
	if !strings.Contains(err.Error(), "phase 5 (") {
		t.Errorf("error = %q, want it to name phase 5", err)
	}
}

// miseLsFailRunner behaves like recorder (returns "{}" for every call, so the
// phase-order tests above keep passing if reused), except any stdin
// containing "mise ls -i -J" gets malformed, non-JSON stdout back — simulating
// stray guest output on that specific read-back.
type miseLsFailRunner struct {
	args [][]string
}

func (r *miseLsFailRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	r.args = append(r.args, args)
	if strings.Contains(string(stdin), "mise ls -i -J") {
		return []byte("not json\n"), nil, nil
	}
	return []byte("{}"), nil, nil
}

// TestPlanSurvivesBrokenMiseListReadback proves a failed `mise ls` metadata
// read-back does not roll back an otherwise-successfully-provisioned VM: by
// this point `mise install` has already succeeded, so only the Record's
// InstalledTools ends up stale, exactly like a failed Record write one layer
// up in cli/create.go. Before this fix, Provisioner.Run returned an error here
// and the caller (cli create/recreate) deleted the just-provisioned VM.
func TestPlanSurvivesBrokenMiseListReadback(t *testing.T) {
	r := &miseLsFailRunner{}
	p := New(lima.New(r), false)
	if err := p.Run(context.Background(), baseResolved(), "/tmp/cfg.yaml"); err != nil {
		t.Fatalf("Run() = %v, want nil (mise ls failure must be non-fatal)", err)
	}
	if got := p.InstalledTools(); len(got) != 0 {
		t.Errorf("InstalledTools() = %v, want empty after a broken mise ls read-back", got)
	}
	// The pipeline must still have reached the final restart.
	var sawRestart bool
	for _, a := range r.args {
		if len(a) > 0 && a[0] == "restart" {
			sawRestart = true
		}
	}
	if !sawRestart {
		t.Errorf("pipeline did not reach restart; ops = %v", r.args)
	}
}

func TestRenderMiseInstall(t *testing.T) {
	got := string(renderMiseInstall([]config.ModuleSpec{{Name: "node", Version: "lts"}}, false))
	for _, want := range []string{
		"export MISE_DATA_DIR=" + miseDataDir,
		"install -d -m 0755 /etc/mise",
		miseConfigPath,
		`"node" = "lts"`,
		"mise install -y\n",
		"mise reshim",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderMiseInstall() does not contain %q:\n%s", want, got)
		}
	}
	if v := string(renderMiseInstall(nil, true)); !strings.Contains(v, "mise install -y --verbose") {
		t.Errorf("verbose install missing --verbose:\n%s", v)
	}
}

// TestEnvContractIsThreeVariables pins the contract shape at its source.
func TestEnvContractIsThreeVariables(t *testing.T) {
	p := New(lima.New(&recorder{}), false)
	env := p.env(baseResolved())
	want := map[string]string{
		"VM_USER":   "me",
		"VM_HOME":   "/home/me.linux",
		"VM_CONFIG": "/mnt/host/vm",
	}
	if len(env) != len(want) {
		t.Fatalf("env = %v, want exactly %v", env, want)
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("env[%q] = %q, want %q", k, env[k], v)
		}
	}
}
