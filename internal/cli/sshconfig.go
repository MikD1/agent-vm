package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/vmname"
	"github.com/spf13/cobra"
)

type sshConfig struct {
	path     string
	warnings io.Writer
}

func newSSHConfig(cmd *cobra.Command) sshConfig {
	warnings := cmd.ErrOrStderr()
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(warnings, "Warning: could not locate SSH config: %v\n", err)
		return sshConfig{}
	}
	return sshConfig{path: filepath.Join(home, ".ssh", "config"), warnings: warnings}
}

func (s sshConfig) add(ctx context.Context, c *lima.Client, name string) {
	if s.path == "" {
		return
	}
	source, err := c.SSHConfigPath(ctx, name)
	if err == nil {
		err = editSSHConfig(s.path, name, source)
	}
	if err != nil {
		fmt.Fprintf(s.warnings, "Warning: could not add SSH Include for %q: %v\n", name, err)
	}
}

func (s sshConfig) remove(name string) {
	if s.path == "" {
		return
	}
	if err := editSSHConfig(s.path, name, ""); err != nil {
		fmt.Fprintf(s.warnings, "Warning: could not remove SSH Include for %q: %v\n", name, err)
	}
}

// OpenSSH parses quotes and backslashes before expanding the Include as a glob.
var sshIncludeEscapes = strings.NewReplacer(`\`, `\\\\`, `"`, `\"`, `*`, `\*`, `?`, `\?`, `[`, `\[`)

// editSSHConfig adds or replaces the VM's Include. An empty source removes it.
func editSSHConfig(path, name, source string) error {
	if err := vmname.Validate(name); err != nil {
		if source == "" {
			return nil // No managed Include can have this name.
		}
		return err
	}
	marker := " # avm: " + name
	var include string
	dir := filepath.Dir(path)
	if source != "" {
		// Newlines break the directive. OpenSSH versions expand percent tokens
		// and environment references in Include paths even inside quotes.
		if strings.ContainsAny(source, "\x00\r\n%") || strings.Contains(source, "${") {
			return fmt.Errorf("unsupported characters in SSH Include path %q", source)
		}
		include = `Include "` + sshIncludeEscapes.Replace(source) + `"` + marker + "\n"
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	// Lock a separate file, which keeps the same inode across config renames.
	lock, err := os.OpenFile(filepath.Join(dir, ".avm.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if include == "" && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	for {
		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("lock SSH config %q: %w", path, err)
	}

	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		path, err = filepath.EvalSymlinks(path)
		if include == "" && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		info, err = os.Stat(path)
	}
	mode := os.FileMode(0o600)
	var original []byte
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return fmt.Errorf("SSH config %q is not a regular file", path)
		}
		mode = info.Mode()
		original, err = os.ReadFile(path)
		if err != nil {
			return err
		}
	case !errors.Is(err, os.ErrNotExist):
		return err
	}

	contents := string(original)
	var updated strings.Builder
	updated.WriteString(include)
	for line := range strings.SplitAfterSeq(contents, "\n") {
		directive, marked := strings.CutSuffix(strings.TrimRight(line, " \t\r\n"), marker)
		directive = strings.Trim(directive, " \t")
		if marked && strings.HasPrefix(directive, `Include "`) && strings.HasSuffix(directive, `"`) {
			continue
		}
		updated.WriteString(line)
	}
	if updated.String() == contents {
		return nil
	}
	return saveSSHConfig(path, updated.String(), mode)
}

func saveSSHConfig(path, contents string, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".avm-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := tmp.WriteString(contents); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
