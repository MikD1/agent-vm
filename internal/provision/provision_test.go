package provision

import (
	"context"
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
	return nil, nil, nil
}

func ops(r *recorder) []string {
	var out []string
	for _, a := range r.args {
		out = append(out, a[0])
	}
	return out
}

func mountResolved() config.Resolved {
	return config.Resolved{
		Name:      "my-api",
		Modules:   []config.ModuleSpec{{Name: "node", Version: "lts"}},
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		User:      "me",
		Workspace: config.Workspace{Mode: "mount", GuestPath: "/home/me.linux/my-api", HostPath: "/h/my-api"},
	}
}

func TestPlanMountOrder(t *testing.T) {
	rec := &recorder{}
	p := New(lima.New(rec), false)
	if err := p.Run(context.Background(), mountResolved(), "/tmp/cfg.yaml"); err != nil {
		t.Fatal(err)
	}
	// create, start, 4 platform provisions (system, base, docker, mise),
	// 1 mise install, then the unconditional restart.
	want := []string{"create", "start", "shell", "shell", "shell", "shell", "shell", "restart"}
	if got := ops(rec); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ops = %v, want %v", got, want)
	}
}

func TestPlanRestartsWithoutDocker(t *testing.T) {
	// Docker is platform, not a module: the restart no longer depends on modules.
	rec := &recorder{}
	r := mountResolved()
	r.Modules = nil
	p := New(lima.New(rec), false)
	if err := p.Run(context.Background(), r, "/tmp/cfg.yaml"); err != nil {
		t.Fatal(err)
	}
	want := []string{"create", "start", "shell", "shell", "shell", "shell", "shell", "restart"}
	if got := ops(rec); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ops = %v, want %v", got, want)
	}
}

func TestPlanCloneAddsClonePhase(t *testing.T) {
	rec := &recorder{}
	r := mountResolved()
	r.Workspace = config.Workspace{Mode: "clone", GuestPath: "/home/me.linux/my-api", Repo: "git@h:a/b.git", Ref: "main"}
	p := New(lima.New(rec), false)
	if err := p.Run(context.Background(), r, "/tmp/cfg.yaml"); err != nil {
		t.Fatal(err)
	}
	// One more shell than the mount case: the clone.
	want := []string{"create", "start", "shell", "shell", "shell", "shell", "shell", "shell", "restart"}
	if got := ops(rec); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ops = %v, want %v", got, want)
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
