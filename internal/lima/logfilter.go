package lima

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
)

// ANSI color codes for the warning/error tags.
const (
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorReset  = "\033[0m"
)

// logFilter is a line-buffered io.Writer over limactl's stderr. It renders
// logrus-formatted lines ("time=... level=... msg=...") into plain text: the
// structured prefix and all trailing key=value fields are dropped, leaving only
// the msg. In normal mode (verbose=false) info/debug lines are suppressed and
// only warnings/errors survive; in verbose mode every line is shown. Lines that
// are not logrus-formatted (e.g. provisioning output) pass through unchanged.
type logFilter struct {
	out     io.Writer
	verbose bool
	color   bool
	buf     []byte
}

// newLogFilter builds a filter writing to out. Color is enabled only when out is
// a character device and NO_COLOR is unset.
func newLogFilter(out io.Writer, verbose bool) *logFilter {
	return &logFilter{
		out:     out,
		verbose: verbose,
		color:   isColorTerminal(out) && os.Getenv("NO_COLOR") == "",
	}
}

func isColorTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func (f *logFilter) Write(p []byte) (int, error) {
	f.buf = append(f.buf, p...)
	for {
		idx := bytes.IndexByte(f.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(f.buf[:idx])
		if err := f.emit(line); err != nil {
			return len(p), err
		}
		f.buf = f.buf[idx+1:]
	}
	return len(p), nil
}

// Flush emits any buffered remainder that did not end in a newline.
func (f *logFilter) Flush() error {
	if len(f.buf) == 0 {
		return nil
	}
	line := string(f.buf)
	f.buf = f.buf[:0]
	return f.emit(line)
}

func (f *logFilter) emit(line string) error {
	line = strings.TrimSuffix(line, "\r")
	out, ok := f.render(line)
	if !ok {
		return nil
	}
	_, err := io.WriteString(f.out, out+"\n")
	return err
}

// render returns the text to print (without trailing newline) and whether to
// print it at all.
func (f *logFilter) render(line string) (string, bool) {
	if !isLogrusLine(line) {
		return line, true
	}
	var level, msg string
	for _, kv := range parseLogrusFields(line) {
		switch kv[0] {
		case "level":
			level = unquote(kv[1])
		case "msg":
			msg = unquote(kv[1])
		}
	}
	switch level {
	case "warning", "warn":
		return f.tagged("warning", colorYellow, msg), true
	case "error", "fatal", "panic":
		return f.tagged("error", colorRed, msg), true
	default: // info, debug, trace, or unknown
		if !f.verbose {
			return "", false
		}
		return msg, true
	}
}

func (f *logFilter) tagged(label, color, msg string) string {
	prefix := label + ":"
	if f.color {
		prefix = color + prefix + colorReset
	}
	if msg == "" {
		return prefix
	}
	return prefix + " " + msg
}

func isLogrusLine(s string) bool {
	return strings.HasPrefix(s, "time=") && strings.Contains(s, " level=")
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' {
		if v, err := strconv.Unquote(s); err == nil {
			return v
		}
	}
	return s
}

// parseLogrusFields splits a logrus text line into ordered key/value pairs.
// Quoted values keep their surrounding quotes (the caller unquotes as needed).
func parseLogrusFields(line string) [][2]string {
	var fields [][2]string
	i, n := 0, len(line)
	for i < n {
		for i < n && line[i] == ' ' {
			i++
		}
		if i >= n {
			break
		}
		keyStart := i
		for i < n && line[i] != '=' && line[i] != ' ' {
			i++
		}
		if i >= n || line[i] != '=' {
			break // not a key=value token
		}
		key := line[keyStart:i]
		i++ // skip '='
		var val string
		if i < n && line[i] == '"' {
			j := i + 1
			for j < n {
				if line[j] == '\\' {
					j += 2
					continue
				}
				if line[j] == '"' {
					break
				}
				j++
			}
			end := j
			if end < n {
				end++ // include the closing quote
			}
			val = line[i:end]
			i = end
		} else {
			valStart := i
			for i < n && line[i] != ' ' {
				i++
			}
			val = line[valStart:i]
		}
		fields = append(fields, [2]string{key, val})
	}
	return fields
}
