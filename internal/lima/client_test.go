package lima

import (
	"context"
	"fmt"
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
	env := map[string]string{"VM_USER": "me", "VM_HOME": "/home/me", "VM_CONFIG": "/mnt/host/vm"}
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

// TestProvisionPassesThreePositionalArgs pins the env contract at the layer
// that actually transports it: the wrapper exports exactly VM_USER, VM_HOME and
// VM_CONFIG, and they arrive as the three trailing positional arguments after
// the `--` separator. A fourth would silently shift $1..$3 in every script.
func TestProvisionPassesThreePositionalArgs(t *testing.T) {
	f := &fakeRunner{}
	c := New(f)
	env := map[string]string{"VM_USER": "me", "VM_HOME": "/home/me", "VM_CONFIG": "/mnt/host/vm"}
	if err := c.Provision(context.Background(), "my-api", []byte("echo hi"), env); err != nil {
		t.Fatal(err)
	}
	args := f.calls[0].args
	if got := args[len(args)-3:]; !equal(got, []string{"me", "/home/me", "/mnt/host/vm"}) {
		t.Errorf("positional env args = %v, want [me /home/me /mnt/host/vm]", got)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, `VM_USER="$1" VM_HOME="$2" VM_CONFIG="$3"`) {
		t.Errorf("wrapper does not export the three-variable contract: %v", args)
	}
	for _, gone := range []string{"VM_PROJECT", "VM_WORKSPACE", "VM_SECRETS"} {
		if strings.Contains(joined, gone) {
			t.Errorf("wrapper still mentions %s: %v", gone, args)
		}
	}
}

func TestEditMountsArgs(t *testing.T) {
	f := &fakeRunner{}
	c := New(f)
	expr := `.mounts = [{"location":"/h/api","mountPoint":"/home/me/api","writable":true}]`
	if err := c.EditMounts(context.Background(), "my-api", expr); err != nil {
		t.Fatal(err)
	}
	want := []string{"edit", "my-api", "--set", expr}
	if got := f.calls[0].args; !equal(got, want) {
		t.Errorf("EditMounts args = %v, want %v", got, want)
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

// TestTailLines: a failing provisioning phase used to quote its whole guest log
// back under the error, even though ExecRunner had already streamed it live.
func TestTailLines(t *testing.T) {
	if got := tailLines("  boom  ", errTailLines); got != "boom" {
		t.Errorf("tailLines(short) = %q, want the text unchanged", got)
	}
	if got := tailLines("a\n\n\nb", errTailLines); got != "a\nb" {
		t.Errorf("tailLines() = %q, want blank lines dropped", got)
	}
	var long []string
	for i := 0; i < errTailLines+5; i++ {
		long = append(long, fmt.Sprintf("line %d", i))
	}
	got := tailLines(strings.Join(long, "\n"), errTailLines)
	if !strings.HasPrefix(got, "(5 earlier lines shown above)\n") {
		t.Errorf("tailLines() must say how much it dropped; got %q", got)
	}
	if !strings.HasSuffix(got, fmt.Sprintf("line %d", errTailLines+4)) {
		t.Errorf("tailLines() must keep the tail, where the cause is; got %q", got)
	}
	if strings.Contains(got, "line 0\n") {
		t.Errorf("tailLines() kept the head: %q", got)
	}
}

// TestProvisionWrapperSourcesTheTrustEnv pins the one thing that makes the
// certificate architecture hold across phases. Phase 1 writes the trust env to
// /etc/profile.d/agent-vm-ca.sh, but every phase reaches the guest as
// "sudo bash -c": not a login shell, so profile.d is never read, and whether
// /etc/environment survives sudo is a property of the guest's PAM configuration,
// not of anything avm sets. Phase 3 — the phase that downloads every tool, and
// the one that reported "invalid peer certificate: UnknownIssuer" on a network
// that inspects TLS — is the furthest from the phase that configured trust.
// Sourcing the file in the wrapper makes the inheritance unconditional.
func TestProvisionWrapperSourcesTheTrustEnv(t *testing.T) {
	f := &fakeRunner{}
	c := New(f)
	env := map[string]string{"VM_USER": "me", "VM_HOME": "/home/me", "VM_CONFIG": "/mnt/host/vm"}
	if err := c.Provision(context.Background(), "my-api", []byte("echo hi"), env); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(f.calls[0].args, " ")
	if !strings.Contains(joined, ". /etc/profile.d/agent-vm-ca.sh") {
		t.Errorf("wrapper does not source the trust env: %v", f.calls[0].args)
	}
	// Phase 1 is the phase that creates the file, so its absence is normal and
	// must never abort a provisioning step.
	if !strings.Contains(joined, "[ -r /etc/profile.d/agent-vm-ca.sh ]") {
		t.Errorf("wrapper sources the trust env unguarded: %v", f.calls[0].args)
	}
}
