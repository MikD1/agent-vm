// Package templates provides the embedded Lima base template and the spec
// template written by `avm init`.
package templates

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

//go:embed files/base.yaml
var BaseLima []byte

// specTemplate is the `avm init` Spec, a text/template whose only placeholder is
// the VM name — init fills it in with the directory's name so the file is
// complete as written, with nothing to uncomment.
//
//go:embed files/agent-vm.yaml
var specTemplate string

// RenderSpec returns the agent-vm.yaml `avm init` writes for a VM named name.
func RenderSpec(name string) ([]byte, error) {
	t, err := template.New("agent-vm.yaml").Parse(specTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse spec template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct{ Name string }{name}); err != nil {
		return nil, fmt.Errorf("render spec template: %w", err)
	}
	return buf.Bytes(), nil
}
