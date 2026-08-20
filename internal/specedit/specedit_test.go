package specedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
	"gopkg.in/yaml.v3"
)

// write puts content into a temp agent-vm.yaml and returns its path.
func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "agent-vm.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// parseMounts re-parses the file as a Spec, proving it stayed valid YAML.
func parseMounts(t *testing.T, p string) []config.MountSpec {
	t.Helper()
	var s config.Spec
	if err := yaml.Unmarshal([]byte(read(t, p)), &s); err != nil {
		t.Fatalf("result is not valid YAML: %v", err)
	}
	return s.Mounts
}

const withComments = `# Tools for this VM.
modules:
  - node: lts

# The projects this VM works on.
mounts:
  - ~/projects/api
`

func TestAddMountKeepsComments(t *testing.T) {
	p := write(t, withComments)
	if err := AddMount(p, config.MountSpec{Path: "~/projects/web"}); err != nil {
		t.Fatal(err)
	}
	got := read(t, p)
	for _, want := range []string{
		"# Tools for this VM.",
		"# The projects this VM works on.",
		"- ~/projects/api",
		"- ~/projects/web",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("AddMount lost %q:\n%s", want, got)
		}
	}
	if mounts := parseMounts(t, p); len(mounts) != 2 {
		t.Errorf("want 2 mounts, got %+v", mounts)
	}
}

func TestAddMountNamedForm(t *testing.T) {
	p := write(t, withComments)
	if err := AddMount(p, config.MountSpec{Path: "~/other/api", Name: "api-legacy"}); err != nil {
		t.Fatal(err)
	}
	mounts := parseMounts(t, p)
	if len(mounts) != 2 {
		t.Fatalf("want 2 mounts, got %+v", mounts)
	}
	if mounts[1].Path != "~/other/api" || mounts[1].Name != "api-legacy" {
		t.Errorf("named form wrong: %+v", mounts[1])
	}
	// The scalar entry must stay scalar, not be rewritten as {path: ...}.
	if strings.Contains(read(t, p), "path: ~/projects/api") {
		t.Errorf("existing scalar entry was rewritten:\n%s", read(t, p))
	}
}

// TestAddMountIntoNullSection is the shape `avm init` writes: a live `mounts:`
// key with no value. yaml.v3 parses that as a !!null SCALAR, not an empty
// sequence — appending to its Content is silently a no-op unless the node is
// converted to a SequenceNode first.
func TestAddMountIntoNullSection(t *testing.T) {
	p := write(t, "modules:\n  - node\n\n# projects go here\nmounts:\n")
	if err := AddMount(p, config.MountSpec{Path: "~/projects/api"}); err != nil {
		t.Fatal(err)
	}
	mounts := parseMounts(t, p)
	if len(mounts) != 1 || mounts[0].Path != "~/projects/api" {
		t.Fatalf("mount was not appended into a null section: %+v", mounts)
	}
	if !strings.Contains(read(t, p), "# projects go here") {
		t.Errorf("comment lost:\n%s", read(t, p))
	}
}

func TestAddMountCreatesMissingSection(t *testing.T) {
	p := write(t, "modules:\n  - node\n")
	if err := AddMount(p, config.MountSpec{Path: "~/projects/api"}); err != nil {
		t.Fatal(err)
	}
	mounts := parseMounts(t, p)
	if len(mounts) != 1 || mounts[0].Path != "~/projects/api" {
		t.Fatalf("mounts section was not created: %+v", mounts)
	}
}

func TestRemoveMount(t *testing.T) {
	p := write(t, "# lead\nmodules:\n  - node\nmounts:\n  - ~/projects/api\n  - path: ~/other/api\n    name: api-legacy\n")
	ok, err := RemoveMount(p, "~/other/api")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("RemoveMount reported no change for an existing entry")
	}
	mounts := parseMounts(t, p)
	if len(mounts) != 1 || mounts[0].Path != "~/projects/api" {
		t.Errorf("wrong entry removed: %+v", mounts)
	}
	if !strings.Contains(read(t, p), "# lead") {
		t.Errorf("comment lost:\n%s", read(t, p))
	}
}

func TestRemoveMountMissingIsNoError(t *testing.T) {
	p := write(t, "mounts:\n  - ~/projects/api\n")
	before := read(t, p)
	ok, err := RemoveMount(p, "~/nope")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("RemoveMount reported a change for a missing entry")
	}
	if read(t, p) != before {
		t.Error("RemoveMount rewrote the file for a missing entry")
	}
}

// TestRoundTripStaysBlockStyle guards the emptied-sequence trap: a sequence
// drained to zero entries re-encodes as flow `mounts: []`, and a later AddMount
// would then produce `mounts: [~/x]` on one line. Style must be reset on insert.
func TestRoundTripStaysBlockStyle(t *testing.T) {
	p := write(t, "mounts:\n  - ~/projects/api\n  - ~/projects/web\n")
	for _, h := range []string{"~/projects/api", "~/projects/web"} {
		if _, err := RemoveMount(p, h); err != nil {
			t.Fatal(err)
		}
	}
	if err := AddMount(p, config.MountSpec{Path: "~/projects/z"}); err != nil {
		t.Fatal(err)
	}
	got := read(t, p)
	if strings.Contains(got, "[") {
		t.Errorf("sequence re-encoded in flow style:\n%s", got)
	}
	if mounts := parseMounts(t, p); len(mounts) != 1 || mounts[0].Path != "~/projects/z" {
		t.Errorf("round trip lost the entry: %+v", mounts)
	}
}

func TestAddMountPreservesFileMode(t *testing.T) {
	p := write(t, "mounts:\n  - ~/projects/api\n")
	if err := AddMount(p, config.MountSpec{Path: "~/projects/web"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode() & os.ModePerm; mode != 0o644 {
		t.Errorf("file mode changed after AddMount: got %o, want 0644", mode)
	}
}

func TestRemoveMountPreservesFileMode(t *testing.T) {
	p := write(t, "mounts:\n  - ~/projects/api\n  - ~/projects/web\n")
	ok, err := RemoveMount(p, "~/projects/api")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("RemoveMount should have removed an entry")
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode() & os.ModePerm; mode != 0o644 {
		t.Errorf("file mode changed after RemoveMount: got %o, want 0644", mode)
	}
}
