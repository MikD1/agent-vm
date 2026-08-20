// Package specedit edits an existing agent-vm.yaml in place, preserving the
// explanatory comments and key order the `avm init` template ships with.
// Editing goes through yaml.Node rather than marshaling a config.Spec: a
// re-marshal would drop every comment in the file.
package specedit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MikD1/agent-vm/internal/config"
	"gopkg.in/yaml.v3"
)

// AddMount appends one entry to the `mounts` sequence, creating the section if
// the file has none. The scalar form is written when Name is empty; otherwise
// the {path, name} mapping form.
func AddMount(path string, m config.MountSpec) error {
	doc, root, err := load(path)
	if err != nil {
		return err
	}
	seq := mountsNode(root, true)
	// A `mounts:` key with no value parses as a !!null scalar, not as an empty
	// sequence — appending to Content would be silently lost. Convert it.
	if seq.Kind != yaml.SequenceNode {
		seq.Kind, seq.Tag, seq.Value, seq.Content = yaml.SequenceNode, "!!seq", "", nil
	}
	// A sequence previously drained to zero entries re-encodes as flow `[]`;
	// clearing Style puts it back into block form.
	seq.Style = 0
	seq.Content = append(seq.Content, mountNode(m))
	return save(path, doc)
}

// RemoveMount deletes the entry whose path equals hostPath. It reports whether
// anything was removed; a missing entry is not an error and leaves the file
// untouched.
func RemoveMount(path, hostPath string) (bool, error) {
	doc, root, err := load(path)
	if err != nil {
		return false, err
	}
	seq := mountsNode(root, false)
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return false, nil
	}
	for i, item := range seq.Content {
		if mountPath(item) != hostPath {
			continue
		}
		seq.Content = append(seq.Content[:i], seq.Content[i+1:]...)
		if err := save(path, doc); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// load parses the file into a document node and returns the root mapping,
// synthesizing both when the file is empty or holds only comments.
func load(path string) (*yaml.Node, *yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read spec: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse spec %s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		doc.Kind = yaml.DocumentNode
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("spec %s is not a YAML mapping", path)
	}
	return &doc, root, nil
}

// mountsNode finds the value node of the `mounts` key, optionally creating the
// key when it is absent.
func mountsNode(root *yaml.Node, create bool) *yaml.Node {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "mounts" {
			return root.Content[i+1]
		}
	}
	if !create {
		return nil
	}
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "mounts"},
		&yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"})
	return root.Content[len(root.Content)-1]
}

// mountNode renders one entry in the shortest form that carries its meaning.
func mountNode(m config.MountSpec) *yaml.Node {
	if m.Name == "" {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: m.Path}
	}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "path"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: m.Path},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: m.Name},
	}}
}

// mountPath reads the host path out of either accepted entry form.
func mountPath(item *yaml.Node) string {
	if item.Kind == yaml.ScalarNode {
		return item.Value
	}
	if item.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(item.Content); i += 2 {
		if item.Content[i].Value == "path" {
			return item.Content[i+1].Value
		}
	}
	return ""
}

// save writes the document atomically (temp file + rename), the same technique
// registry.Store.Write uses, so a crash cannot leave a half-written spec.
// File mode is preserved from the original file; if the file does not yet exist,
// it defaults to 0o644 (readable by all, writable by owner).
func save(path string, doc *yaml.Node) error {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}

	// Determine the original file's mode, or use a sensible default.
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode()
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".agent-vm.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(b.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
