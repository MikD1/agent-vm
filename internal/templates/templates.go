// Package templates provides the embedded Lima base template and the spec
// template written by `avm init`.
package templates

import _ "embed"

//go:embed files/base.yaml
var BaseLima []byte

//go:embed files/agent-vm.yaml
var SpecTemplate []byte
