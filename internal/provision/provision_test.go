package provision

import (
	"context"
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/lima"
)

type recorder struct{ args [][]string }

func (r *recorder) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	r.args = append(r.args, args)
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
		Modules:   []string{"node", "docker"},
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		User:      "me",
		Workspace: config.Workspace{Mode: "mount", GuestPath: "/home/me.linux/my-api", HostPath: "/h/my-api"},
	}
}

func TestPlanMountOrderAndDockerRestart(t *testing.T) {
	rec := &recorder{}
	p := New(lima.New(rec), "")
	err := p.Run(context.Background(), mountResolved(), "/tmp/cfg.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got := ops(rec)
	// create, start, then 4 provision (system, base, node, docker), then restart (docker).
	want := []string{"create", "start", "shell", "shell", "shell", "shell", "restart"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ops = %v, want %v", got, want)
	}
}

func TestPlanNoRestartWithoutDocker(t *testing.T) {
	rec := &recorder{}
	r := mountResolved()
	r.Modules = []string{"node"}
	p := New(lima.New(rec), "")
	_ = p.Run(context.Background(), r, "/tmp/cfg.yaml")
	got := ops(rec)
	want := []string{"create", "start", "shell", "shell", "shell"} // system, base, node; no restart
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ops = %v, want %v", got, want)
	}
}

func TestPlanCloneAddsClonePhase(t *testing.T) {
	rec := &recorder{}
	r := mountResolved()
	r.Modules = []string{"node"}
	r.Workspace = config.Workspace{Mode: "clone", GuestPath: "/home/me.linux/my-api", Repo: "git@h:a/b.git", Ref: "main"}
	p := New(lima.New(rec), "")
	_ = p.Run(context.Background(), r, "/tmp/cfg.yaml")
	got := ops(rec)
	// create, start, system, base, node, clone(shell). 6 calls, last is shell.
	want := []string{"create", "start", "shell", "shell", "shell", "shell"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ops = %v, want %v", got, want)
	}
}
