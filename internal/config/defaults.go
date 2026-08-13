package config

// Built-in defaults (lowest precedence in Resolve).
const (
	DefaultCPUs   = 4
	DefaultMemory = "4GiB"
	DefaultDisk   = "120GiB"
	DefaultImage  = "template:_images/ubuntu"
)

// DefaultToolVersion is what a module named without a version resolves to.
// Pinning is the documented practice; the `avm init` template pins Node.
const DefaultToolVersion = "latest"

// DefaultModules apply only when the Spec has no `modules` key at all.
var DefaultModules = []ModuleSpec{{Name: "node", Version: "lts"}, {Name: "claude"}}

// DefaultFileMode is applied to a copied file when the Spec sets no mode.
// Directory sources keep the modes cp -R preserves.
const DefaultFileMode = "0644"
