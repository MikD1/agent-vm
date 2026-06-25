package lima

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var execCommandContext = exec.CommandContext

// Client wraps limactl invocations behind a CommandRunner.
type Client struct{ runner CommandRunner }

// New builds a Client over the given runner.
func New(r CommandRunner) *Client { return &Client{runner: r} }

func (c *Client) run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	out, errb, err := c.runner.Run(ctx, stdin, args...)
	if err != nil {
		return out, fmt.Errorf("limactl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(errb)))
	}
	return out, nil
}

// Instance describes a Lima VM returned by `limactl list`.
type Instance struct {
	Name  string
	State string
}

// Instances lists existing Lima VMs with their runtime state.
func (c *Client) Instances(ctx context.Context) ([]Instance, error) {
	out, err := c.run(ctx, nil, "list", "--format", "{{.Name}}\t{{.Status}}")
	if err != nil {
		return nil, err
	}
	var instances []Instance
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		name, state, _ := strings.Cut(line, "\t")
		if name == "" {
			continue
		}
		instances = append(instances, Instance{Name: name, State: normalizeState(state)})
	}
	return instances, nil
}

// Names lists existing Lima VM names.
func (c *Client) Names(ctx context.Context) ([]string, error) {
	instances, err := c.Instances(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(instances))
	for _, inst := range instances {
		names = append(names, inst.Name)
	}
	return names, nil
}

func normalizeState(state string) string {
	state = strings.ToLower(strings.TrimSpace(state))
	if state == "" {
		return "-"
	}
	return state
}

// InfoRaw returns the raw JSON from `limactl info` for the caller to parse.
func (c *Client) InfoRaw(ctx context.Context) ([]byte, error) {
	return c.run(ctx, nil, "info")
}

func (c *Client) Create(ctx context.Context, name, configPath string) error {
	_, err := c.run(ctx, nil, "create", "--name="+name, "--tty=false", configPath)
	return err
}

func (c *Client) Start(ctx context.Context, name string) error {
	_, err := c.run(ctx, nil, "start", name)
	return err
}

func (c *Client) Stop(ctx context.Context, name string) error {
	_, err := c.run(ctx, nil, "stop", name)
	return err
}

func (c *Client) Restart(ctx context.Context, name string) error {
	_, err := c.run(ctx, nil, "restart", name)
	return err
}

// Delete force-removes a VM (no error if absent is handled by the caller).
func (c *Client) Delete(ctx context.Context, name string) error {
	_, err := c.run(ctx, nil, "delete", "-f", name)
	return err
}

// Provision streams a script to the guest as root with the env contract exported.
// Env is passed via positional args to avoid quoting issues.
func (c *Client) Provision(ctx context.Context, name string, script []byte, env map[string]string) error {
	wrapper := `export VM_USER="$1" VM_PROJECT="$2" VM_WORKSPACE="$3" VM_SECRETS="$4"
export DEBIAN_FRONTEND=noninteractive
exec bash -euo pipefail -s`
	args := []string{
		"shell", "--workdir", "/", name,
		"sudo", "bash", "-c", wrapper, "--",
		env["VM_USER"], env["VM_PROJECT"], env["VM_WORKSPACE"], env["VM_SECRETS"],
	}
	_, err := c.run(ctx, script, args...)
	return err
}

// Shell runs an interactive shell in the VM at workdir (empty workdir = default).
// It connects the process stdio directly (not via CommandRunner, which buffers).
func (c *Client) Shell(ctx context.Context, name, workdir string, extra ...string) error {
	args := []string{"shell"}
	if workdir != "" {
		args = append(args, "--workdir", workdir)
	}
	args = append(args, name)
	args = append(args, extra...)
	cmd := execCommandContext(ctx, "limactl", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
