package lima

import (
	"bytes"
	"testing"
)

// runFilter feeds the given writes through a logFilter (no color: the sink is a
// *bytes.Buffer, not a char device), flushes, and returns what was rendered.
func runFilter(t *testing.T, verbose bool, writes ...string) string {
	t.Helper()
	var buf bytes.Buffer
	f := newLogFilter(&buf, verbose)
	for _, w := range writes {
		if _, err := f.Write([]byte(w)); err != nil {
			t.Fatalf("Write(%q): %v", w, err)
		}
	}
	if err := f.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	return buf.String()
}

func TestLogFilter(t *testing.T) {
	tests := []struct {
		name        string
		verbose     bool
		writes      []string
		want        string
	}{
		{
			name:    "info dropped in normal mode",
			verbose: false,
			writes:  []string{"time=\"t\" level=info msg=\"hello world\"\n"},
			want:    "",
		},
		{
			name:    "info shown bare in verbose mode",
			verbose: true,
			writes:  []string{"time=\"t\" level=info msg=\"hello world\"\n"},
			want:    "hello world\n",
		},
		{
			name:    "warning tagged in normal mode",
			verbose: false,
			writes:  []string{"time=\"t\" level=warning msg=\"watch out\"\n"},
			want:    "warning: watch out\n",
		},
		{
			name:    "warning tagged in verbose mode",
			verbose: true,
			writes:  []string{"time=\"t\" level=warning msg=\"watch out\"\n"},
			want:    "warning: watch out\n",
		},
		{
			name:    "error tagged",
			verbose: false,
			writes:  []string{"time=\"t\" level=error msg=\"bad\"\n"},
			want:    "error: bad\n",
		},
		{
			name:    "fatal renders as error",
			verbose: false,
			writes:  []string{"time=\"t\" level=fatal msg=\"dead\"\n"},
			want:    "error: dead\n",
		},
		{
			name:    "non-logrus line passes through",
			verbose: false,
			writes:  []string{"installing package foo\n"},
			want:    "installing package foo\n",
		},
		{
			name:    "trailing structured fields dropped",
			verbose: true,
			writes:  []string{"time=\"t\" level=info msg=\"downloading\" arch=aarch64 location=\"https://e/x.img\"\n"},
			want:    "downloading\n",
		},
		{
			name:    "quoted msg with escapes is unquoted",
			verbose: false,
			writes:  []string{"time=\"t\" level=warning msg=\"user \\\"bob\\\" invalid\"\n"},
			want:    "warning: user \"bob\" invalid\n",
		},
		{
			name:    "logrus line split across writes",
			verbose: false,
			writes:  []string{"time=\"t\" level=warning msg=\"par", "tial line\"\n"},
			want:    "warning: partial line\n",
		},
		{
			name:    "unterminated final line flushed",
			verbose: false,
			writes:  []string{"time=\"t\" level=error msg=\"no newline\""},
			want:    "error: no newline\n",
		},
		{
			name:    "info dropped but warning kept in same write",
			verbose: false,
			writes: []string{
				"time=\"t\" level=info msg=\"chatter\"\ntime=\"t\" level=warning msg=\"heads up\"\n",
			},
			want: "warning: heads up\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runFilter(t, tt.verbose, tt.writes...)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
