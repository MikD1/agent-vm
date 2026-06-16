// Package lima is the only package that shells out to limactl. Everything above
// it depends on Client, which is driven by an injectable CommandRunner.
package lima

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
)

// CommandRunner executes `limactl <args...>` with optional stdin.
type CommandRunner interface {
	Run(ctx context.Context, stdin []byte, args ...string) (stdout, stderr []byte, err error)
}

// ExecRunner runs the real limactl binary. It streams limactl's stderr
// directly to os.Stderr so Lima's own progress (image download, VM start)
// is visible in the terminal.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "limactl", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	// MultiWriter: capture stderr for error messages AND stream to terminal.
	cmd.Stderr = io.MultiWriter(os.Stderr, &errb)
	err := cmd.Run()
	return out.Bytes(), errb.Bytes(), err
}
