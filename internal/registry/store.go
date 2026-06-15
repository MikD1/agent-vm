package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Store is the on-disk registry rooted at <root>/vms.
type Store struct{ root string }

// NewStore roots a Store at the given host config dir (e.g. ~/.config/agent-vm).
func NewStore(root string) *Store { return &Store{root: root} }

// DefaultRoot resolves ~/.config/agent-vm honoring XDG_CONFIG_HOME.
func DefaultRoot() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "agent-vm"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".config", "agent-vm"), nil
}

func (s *Store) vmsDir() string          { return filepath.Join(s.root, "vms") }
func (s *Store) path(name string) string { return filepath.Join(s.vmsDir(), name+".yaml") }

// Write atomically persists a Record (temp file + rename).
func (s *Store) Write(r Record) error {
	if err := os.MkdirAll(s.vmsDir(), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.vmsDir(), "."+r.Name+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path(r.Name))
}

// Read loads one Record by name.
func (s *Store) Read(name string) (Record, error) {
	data, err := os.ReadFile(s.path(name))
	if err != nil {
		return Record{}, fmt.Errorf("read record %q: %w", name, err)
	}
	var r Record
	if err := yaml.Unmarshal(data, &r); err != nil {
		return Record{}, fmt.Errorf("parse record %q: %w", name, err)
	}
	return r, nil
}

// Exists reports whether a Record file exists for name.
func (s *Store) Exists(name string) (bool, error) {
	_, err := os.Stat(s.path(name))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// Delete removes a Record (no error if already absent).
func (s *Store) Delete(name string) error {
	err := os.Remove(s.path(name))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// List returns all Records sorted by name.
func (s *Store) List() ([]Record, error) {
	entries, err := os.ReadDir(s.vmsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		r, err := s.Read(strings.TrimSuffix(e.Name(), ".yaml"))
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
