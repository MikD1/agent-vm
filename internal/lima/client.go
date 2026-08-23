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

// errTailLines caps how much of limactl's stderr an error repeats. ExecRunner
// already rendered that stream live through the logFilter, so quoting all of it
// prints the failure twice: a phase 3 that dies on the last of five tools would
// otherwise put mise's entire log under the error message, burying the one line
// that says what went wrong. The tail is where the cause is.
const errTailLines = 12

// tailLines keeps the last n non-blank lines of s, noting how many it dropped.
func tailLines(s string, n int) string {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return fmt.Sprintf("(%d earlier lines shown above)\n%s",
		len(lines)-n, strings.Join(lines[len(lines)-n:], "\n"))
}

func (c *Client) run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	out, errb, err := c.runner.Run(ctx, stdin, args...)
	if err != nil {
		return out, fmt.Errorf("limactl %s: %w: %s", strings.Join(args, " "), err, tailLines(string(errb), errTailLines))
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

// provisionWrapper re-exports the guest env contract from positional args, so no
// value ever passes through shell quoting. Both provisioning entry points share
// it: the contract must not drift between them.
const provisionWrapper = `export VM_USER="$1" VM_HOME="$2" VM_CONFIG="$3"
export DEBIAN_FRONTEND=noninteractive
exec bash -euo pipefail -s`

// provisionArgs builds the limactl argv for one provisioning call. The three
// contract values are passed positionally, in the order provisionWrapper reads
// them.
func provisionArgs(name string, env map[string]string) []string {
	return []string{
		"shell", "--workdir", "/", name,
		"sudo", "bash", "-c", provisionWrapper, "--",
		env["VM_USER"], env["VM_HOME"], env["VM_CONFIG"],
	}
}

// Provision streams a script to the guest as root with the env contract exported.
func (c *Client) Provision(ctx context.Context, name string, script []byte, env map[string]string) error {
	_, err := c.run(ctx, script, provisionArgs(name, env)...)
	return err
}

// ProvisionOutput is Provision plus the guest's stdout, for scripts whose output
// avm needs to parse.
func (c *Client) ProvisionOutput(ctx context.Context, name string, script []byte, env map[string]string) ([]byte, error) {
	return c.run(ctx, script, provisionArgs(name, env)...)
}

// EditMounts rewrites the instance's mount list via a yq expression. Lima
// attaches virtiofs devices at boot, so the caller stops and starts the VM
// around this call for the change to take effect.
func (c *Client) EditMounts(ctx context.Context, name, yqExpr string) error {
	_, err := c.run(ctx, nil, "edit", name, "--set", yqExpr)
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
