package config

// Built-in defaults (lowest precedence in Resolve).
const (
	DefaultCPUs   = 4
	DefaultMemory = "4GiB"
	DefaultDisk   = "120GiB"
	DefaultImage  = "template:_images/ubuntu"
	ModeMount     = "mount"
	ModeClone     = "clone"
	DefaultRef    = "main"
)

// DefaultModules apply only when no module information exists anywhere
// (clone from a bare repo with no in-repo spec). claude needs npm from node, so
// node comes first.
var DefaultModules = []string{"node", "claude"}
