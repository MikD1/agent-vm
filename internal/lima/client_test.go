package lima

import (
	"context"
	"strings"
	"testing"
)

type call struct {
	args  []string
	stdin string
}

type fakeRunner struct {
	calls  []call
	stdout map[string][]byte // keyed by strings.Join(args, " ")
	err    error
}

func (f *fakeRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, call{args: args, stdin: string(stdin)})
	if f.err != nil {
		return nil, []byte("boom"), f.err
	}
	return f.stdout[strings.Join(args, " ")], nil, nil
}

func TestNames(t *testing.T) {
	f := &fakeRunner{stdout: map[string][]byte{
		"list --format {{.Name}}\t{{.Status}}": []byte("alpha\trunning\nbeta\tstopped\n"),
	}}
	c := New(f)
	names, err := c.Names(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("names = %v", names)
	}
}

func TestInstances(t *testing.T) {
	f := &fakeRunner{stdout: map[string][]byte{
		"list --format {{.Name}}\t{{.Status}}": []byte("alpha\tRunning\nbeta\tStopped\n"),
	}}
	c := New(f)
	instances, err := c.Instances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 2 {
		t.Fatalf("instances = %v", instances)
	}
	if instances[0].Name != "alpha" || instances[0].State != "running" {
		t.Errorf("first instance = %+v", instances[0])
	}
	if instances[1].Name != "beta" || instances[1].State != "stopped" {
		t.Errorf("second instance = %+v", instances[1])
	}
}

func TestCreateArgs(t *testing.T) {
	f := &fakeRunner{}
	c := New(f)
	if err := c.Create(context.Background(), "my-api", "/tmp/cfg.yaml"); err != nil {
		t.Fatal(err)
	}
	want := []string{"create", "--name=my-api", "--tty=false", "/tmp/cfg.yaml"}
	if got := f.calls[0].args; !equal(got, want) {
		t.Errorf("create args = %v, want %v", got, want)
	}
}

func TestProvisionStdin(t *testing.T) {
	f := &fakeRunner{}
	c := New(f)
	env := map[string]string{"VM_USER": "me", "VM_PROJECT": "my-api", "VM_WORKSPACE": "/home/me/my-api", "VM_SECRETS": "/mnt/host/agent-vm"}
	if err := c.Provision(context.Background(), "my-api", []byte("echo hi"), env); err != nil {
		t.Fatal(err)
	}
	got := f.calls[0]
	if got.args[0] != "shell" || got.stdin != "echo hi" {
		t.Errorf("provision call = %+v", got)
	}
	joined := strings.Join(got.args, " ")
	if !strings.Contains(joined, "--workdir /") || !strings.Contains(joined, "sudo") {
		t.Errorf("provision args missing workdir/sudo: %v", got.args)
	}
}

func TestRunError(t *testing.T) {
	f := &fakeRunner{err: context.DeadlineExceeded}
	c := New(f)
	if err := c.Start(context.Background(), "x"); err == nil {
		t.Error("want error propagated from runner")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
