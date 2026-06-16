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

// ExecRunner runs the real limactl binary. limactl's logrus-formatted stderr is
// passed through a logFilter before reaching the terminal: in normal mode only
// warnings/errors survive, with Verbose every line is shown — both with the
// structured prefix and trailing fields stripped. The raw stderr is still
// captured separately for error messages.
type ExecRunner struct {
	Verbose bool      // true → full log; false → warnings/errors only
	Out     io.Writer // filtered output sink; nil → os.Stderr
}

func (r ExecRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "limactl", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	sink := r.Out
	if sink == nil {
		sink = os.Stderr
	}
	filter := newLogFilter(sink, r.Verbose)
	// errb keeps the raw stderr (used to build error messages); filter renders
	// the cleaned output to the terminal.
	cmd.Stderr = io.MultiWriter(&errb, filter)
	err := cmd.Run()
	filter.Flush()
	return out.Bytes(), errb.Bytes(), err
}
