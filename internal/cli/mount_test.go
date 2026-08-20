package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/lima"
	"github.com/MikD1/agent-vm/internal/registry"
	"github.com/MikD1/agent-vm/internal/vmname"
	"gopkg.in/yaml.v3"
)

// stateRunner reports a fixed `limactl list` result and records every op, so a
// test can assert the exact stop/edit/start sequence a mount triggers.
type stateRunner struct {
	state string // "" → the VM is absent from Lima
	ops   []string
	sets  []string // the yq expressions passed to `limactl edit --set`
}

func (s *stateRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	s.ops = append(s.ops, args[0])
	switch args[0] {
	case "list":
		if s.state == "" {
			return []byte(""), nil, nil
		}
		return []byte("work\t" + s.state + "\n"), nil, nil
	case "stop":
		s.state = "stopped"
	case "start":
		s.state = "running"
	case "edit":
		// Real Lima refuses to edit a running instance: the VM must be stopped
		// first. Mirroring that here is what keeps the stop → edit → start order
		// enforceable by the test suite.
		if s.state == "running" {
			return nil, []byte("cannot edit a running instance"), errors.New("cannot edit a running instance")
		}
		for i, a := range args {
			if a == "--set" && i+1 < len(args) {
				s.sets = append(s.sets, args[i+1])
			}
		}
	}
	return nil, nil, nil
}

// vmDir builds a VM directory with a spec, plus a project folder to mount, and
// returns (configDir, projectPath, store).
func vmDir(t *testing.T, specBody string) (string, string, *registry.Store) {
	t.Helper()
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "agent-vm.yaml"), []byte(specBody), 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "api")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	return configDir, project, registry.NewStore(t.TempDir())
}

func record(configDir string, mounts ...config.Mount) registry.Record {
	return registry.Record{
		Name: "work", User: "me",
		ConfigDir: configDir,
		Home:      "/home/me.linux",
		Mounts:    mounts,
	}
}

func specMounts(t *testing.T, configDir string) []config.MountSpec {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(configDir, "agent-vm.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var s config.Spec
	if err := yaml.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s.Mounts
}

// TestMountStoppedVMEditsOnly: a stopped VM needs no restart dance, just the
// config rewrite.
func TestMountStoppedVMEditsOnly(t *testing.T) {
	configDir, project, store := vmDir(t, "modules:\n  - node\nmounts:\n")
	_ = store.Write(record(configDir))
	r := &stateRunner{state: "stopped"}
	deps := createDeps{lima: lima.New(r), store: store}

	if err := runMount(context.Background(), deps, "work", project, ""); err != nil {
		t.Fatal(err)
	}
	var edits, lifecycle int
	for _, op := range r.ops {
		switch op {
		case "edit":
			edits++
		case "stop", "start":
			lifecycle++
		}
	}
	if edits != 1 {
		t.Errorf("want exactly one edit, ops = %v", r.ops)
	}
	if lifecycle != 0 {
		t.Errorf("a stopped VM must not be stopped or started, ops = %v", r.ops)
	}
	// All three artifacts updated.
	if got := specMounts(t, configDir); len(got) != 1 || got[0].Path != project {
		t.Errorf("spec mounts = %+v, want the project", got)
	}
	rec, err := store.Read("work")
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Mounts) != 1 || rec.Mounts[0].GuestPath != "/home/me.linux/api" {
		t.Errorf("record mounts = %+v", rec.Mounts)
	}
	if len(r.sets) != 1 || !strings.HasPrefix(r.sets[0], ".mounts = [") {
		t.Errorf("yq expression = %v", r.sets)
	}
	// The service mount is always rendered first, read-only.
	if !strings.Contains(r.sets[0], `"mountPoint":"/mnt/host/vm"`) {
		t.Errorf("yq expression is missing the service mount: %s", r.sets[0])
	}
}

// TestMountAbsentVMTouchesNoLima: a Record whose VM was never built (or was
// deleted out of band) still gets its config and Record updated, with no Lima
// calls beyond the existence check.
func TestMountAbsentVMTouchesNoLima(t *testing.T) {
	configDir, project, store := vmDir(t, "mounts:\n")
	_ = store.Write(record(configDir))
	r := &stateRunner{state: ""}
	deps := createDeps{lima: lima.New(r), store: store}

	if err := runMount(context.Background(), deps, "work", project, ""); err != nil {
		t.Fatal(err)
	}
	for _, op := range r.ops {
		if op == "edit" || op == "stop" || op == "start" {
			t.Errorf("no VM exists, so Lima must not be mutated; ops = %v", r.ops)
		}
	}
	if got := specMounts(t, configDir); len(got) != 1 {
		t.Errorf("spec must still be updated: %+v", got)
	}
}

// TestMountAlreadyMountedIsNoOp pins acceptance criterion 7.
func TestMountAlreadyMountedIsNoOp(t *testing.T) {
	configDir, project, store := vmDir(t, "mounts:\n  - "+"placeholder"+"\n")
	_ = store.Write(record(configDir, config.Mount{HostPath: project, GuestPath: "/home/me.linux/api"}))
	r := &stateRunner{state: "running"}
	deps := createDeps{lima: lima.New(r), store: store}

	if err := runMount(context.Background(), deps, "work", project, ""); err != nil {
		t.Fatal(err)
	}
	for _, op := range r.ops {
		if op == "edit" || op == "stop" || op == "start" {
			t.Errorf("re-mounting the same path must change nothing; ops = %v", r.ops)
		}
	}
	rec, _ := store.Read("work")
	if len(rec.Mounts) != 1 {
		t.Errorf("record must keep exactly one mount, got %+v", rec.Mounts)
	}
}

// withStdin points os.Stdin at a file holding input for the rest of the test.
// fmt.Scanln resolves os.Stdin at call time, so this is enough to answer the
// confirmation prompt; without it `go test` always declines (real stdin is
// empty or closed).
func withStdin(t *testing.T, input string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = orig; _ = f.Close() })
}

// TestMountRunningVMDeclineTouchesNoLima pins the running-instance constraint:
// `limactl edit` refuses to operate on a running VM, so the prompt must come
// before any Lima mutation. Declining leaves Lima entirely untouched — no edit
// is even attempted, because it would fail and there is nothing a running VM
// could pick up from it anyway.
func TestMountRunningVMDeclineTouchesNoLima(t *testing.T) {
	configDir, project, store := vmDir(t, "mounts:\n")
	_ = store.Write(record(configDir))
	r := &stateRunner{state: "running"}
	deps := createDeps{lima: lima.New(r), store: store}
	withStdin(t, "\n") // decline

	if err := runMount(context.Background(), deps, "work", project, ""); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.ops, ","); got != "list" {
		t.Errorf("declining must issue only the state check, ops = %v", r.ops)
	}
	if len(r.sets) != 0 {
		t.Errorf("declining must not rewrite the Lima config, sets = %v", r.sets)
	}
	// The other two artifacts are still updated, which is what the decline
	// message tells the user.
	if got := specMounts(t, configDir); len(got) != 1 {
		t.Errorf("spec must still be updated: %+v", got)
	}
	rec, _ := store.Read("work")
	if len(rec.Mounts) != 1 {
		t.Errorf("record must still be updated: %+v", rec.Mounts)
	}
}

// TestMountRunningVMAcceptStopsThenEdits pins the only order real Lima accepts:
// stop, then edit, then start. The fake rejects an edit while its state is
// "running", so this also proves no edit is attempted before the stop.
func TestMountRunningVMAcceptStopsThenEdits(t *testing.T) {
	configDir, project, store := vmDir(t, "mounts:\n")
	_ = store.Write(record(configDir))
	r := &stateRunner{state: "running"}
	deps := createDeps{lima: lima.New(r), store: store}
	withStdin(t, "y\n")

	if err := runMount(context.Background(), deps, "work", project, ""); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(r.ops, ","), "list,stop,edit,start"; got != want {
		t.Errorf("ops = %v, want %s", r.ops, want)
	}
	if len(r.sets) != 1 || !strings.HasPrefix(r.sets[0], ".mounts = [") {
		t.Errorf("yq expression = %v", r.sets)
	}
	if r.state != "running" {
		t.Errorf("the VM must be left running, state = %q", r.state)
	}
}

// TestMountRunningVMEditFailureIsReported: a sync that fails cannot be retried
// by re-running `avm mount` (the Record already has the entry), so the error
// must say the other two artifacts are already updated.
func TestMountRunningVMEditFailureIsReported(t *testing.T) {
	configDir, project, store := vmDir(t, "mounts:\n")
	_ = store.Write(record(configDir))
	r := &failingRunner{stateRunner: stateRunner{state: "stopped"}}
	deps := createDeps{lima: lima.New(r), store: store}

	err := runMount(context.Background(), deps, "work", project, "")
	if err == nil {
		t.Fatal("want the sync failure to surface")
	}
	if !strings.Contains(err.Error(), "the record are updated") {
		t.Errorf("error = %q, want it to say the spec and record are already written", err)
	}
}

// failingRunner fails every `limactl edit`, standing in for a transient Lima
// error unrelated to run state.
type failingRunner struct{ stateRunner }

func (f *failingRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	if args[0] == "edit" {
		f.ops = append(f.ops, args[0])
		return nil, []byte("boom"), errors.New("boom")
	}
	return f.stateRunner.Run(ctx, stdin, args...)
}

// TestMountGuestPathCollision pins acceptance criterion 8's failure half.
func TestMountGuestPathCollision(t *testing.T) {
	configDir, project, store := vmDir(t, "mounts:\n")
	// An unrelated host folder already occupies ~/api in the guest.
	_ = store.Write(record(configDir, config.Mount{HostPath: "/elsewhere/api", GuestPath: "/home/me.linux/api"}))
	deps := createDeps{lima: lima.New(&stateRunner{state: "stopped"}), store: store}

	err := runMount(context.Background(), deps, "work", project, "")
	if err == nil {
		t.Fatal("want a collision error")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Errorf("collision error = %q, want it to suggest --name", err)
	}
}

// TestMountWithNameWritesBothArtifacts is criterion 8's success half.
func TestMountWithNameWritesBothArtifacts(t *testing.T) {
	configDir, project, store := vmDir(t, "mounts:\n")
	_ = store.Write(record(configDir, config.Mount{HostPath: "/elsewhere/api", GuestPath: "/home/me.linux/api"}))
	deps := createDeps{lima: lima.New(&stateRunner{state: "stopped"}), store: store}

	if err := runMount(context.Background(), deps, "work", project, "api-legacy"); err != nil {
		t.Fatal(err)
	}
	got := specMounts(t, configDir)
	if len(got) != 1 || got[0].Name != "api-legacy" {
		t.Errorf("spec must record the explicit name: %+v", got)
	}
	rec, _ := store.Read("work")
	if len(rec.Mounts) != 2 || rec.Mounts[1].GuestPath != "/home/me.linux/api-legacy" {
		t.Errorf("record mounts = %+v", rec.Mounts)
	}
}

func TestMountMissingRecord(t *testing.T) {
	_, project, store := vmDir(t, "mounts:\n")
	deps := createDeps{lima: lima.New(&stateRunner{}), store: store}
	err := runMount(context.Background(), deps, "ghost", project, "")
	if err == nil || !strings.Contains(err.Error(), "no VM named") {
		t.Errorf("error = %v, want `no VM named`", err)
	}
}

func TestMountMissingVMDirectory(t *testing.T) {
	_, project, store := vmDir(t, "mounts:\n")
	_ = store.Write(record(filepath.Join(t.TempDir(), "gone")))
	deps := createDeps{lima: lima.New(&stateRunner{}), store: store}
	err := runMount(context.Background(), deps, "work", project, "")
	if err == nil || !strings.Contains(err.Error(), "is missing") {
		t.Errorf("error = %v, want a missing-directory error", err)
	}
}

func TestMountRejectsNonDirectory(t *testing.T) {
	configDir, _, store := vmDir(t, "mounts:\n")
	file := filepath.Join(configDir, "agent-vm.yaml")
	_ = store.Write(record(configDir))
	deps := createDeps{lima: lima.New(&stateRunner{}), store: store}
	if err := runMount(context.Background(), deps, "work", file, ""); err == nil {
		t.Error("mounting a file must fail")
	}
}

func TestUnmountByHostPathAndByName(t *testing.T) {
	for _, tt := range []struct {
		name   string
		target func(project string) string
	}{
		{"by host path", func(p string) string { return p }},
		{"by guest name", func(string) string { return "api" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configDir, project, store := vmDir(t, "mounts:\n  - "+"x"+"\n")
			_ = store.Write(record(configDir, config.Mount{HostPath: project, GuestPath: "/home/me.linux/api"}))
			// Put the real path into the spec so RemoveMount has something to drop.
			if err := os.WriteFile(filepath.Join(configDir, "agent-vm.yaml"),
				[]byte("mounts:\n  - "+project+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			r := &stateRunner{state: "stopped"}
			deps := createDeps{lima: lima.New(r), store: store}

			if err := runUnmount(context.Background(), deps, "work", tt.target(project)); err != nil {
				t.Fatal(err)
			}
			rec, _ := store.Read("work")
			if len(rec.Mounts) != 0 {
				t.Errorf("record still has mounts: %+v", rec.Mounts)
			}
			if got := specMounts(t, configDir); len(got) != 0 {
				t.Errorf("spec still has mounts: %+v", got)
			}
		})
	}
}

func TestUnmountMissingTarget(t *testing.T) {
	configDir, _, store := vmDir(t, "mounts:\n")
	_ = store.Write(record(configDir))
	deps := createDeps{lima: lima.New(&stateRunner{state: "stopped"}), store: store}
	err := runUnmount(context.Background(), deps, "work", "nope")
	if err == nil || !strings.Contains(err.Error(), "is not mounted") {
		t.Errorf("error = %v, want `is not mounted`", err)
	}
}

// TestUnmountSpecMismatchErrors pins the fix for the silently-stale-spec bug:
// if the spec spells the mount in a form that matches neither the ~/ form nor
// the absolute form of the Record's HostPath (e.g. it was hand-edited to some
// unrelated string), RemoveMount finds nothing in either attempt, and
// runUnmount must report that instead of claiming success while the spec keeps
// the stale entry (which `avm recreate` would later resurrect).
func TestUnmountSpecMismatchErrors(t *testing.T) {
	configDir, project, store := vmDir(t, "mounts:\n  - completely-unrelated-entry\n")
	_ = store.Write(record(configDir, config.Mount{HostPath: project, GuestPath: "/home/me.linux/api"}))
	deps := createDeps{lima: lima.New(&stateRunner{state: "stopped"}), store: store}

	err := runUnmount(context.Background(), deps, "work", project)
	if err == nil {
		t.Fatal("want an error when the spec has no matching entry to remove")
	}
	if !strings.Contains(err.Error(), "not found in agent-vm.yaml") {
		t.Errorf("error = %q, want it to mention the spec still has a stale entry", err)
	}
	// The Record must be left untouched too: a half-applied unmount would let
	// the two artifacts drift.
	rec, _ := store.Read("work")
	if len(rec.Mounts) != 1 {
		t.Errorf("record must be unchanged on this error, got %+v", rec.Mounts)
	}
	if got := specMounts(t, configDir); len(got) != 1 || got[0].Path != "completely-unrelated-entry" {
		t.Errorf("spec must be unchanged on this error, got %+v", got)
	}
}

// TestMountRejectsVMOwnDirectory: the VM directory is already mounted read-only
// at /mnt/host/vm. Mounting it (or a parent of it) again would add a second,
// read/write mount of the same host path — Lima applies both — handing the guest
// write access to agent-vm.yaml, the CA bundle, and every declared credential.
func TestMountRejectsVMOwnDirectory(t *testing.T) {
	parent := t.TempDir()
	configDir := filepath.Join(parent, "work")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "agent-vm.yaml"), []byte("mounts:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := registry.NewStore(t.TempDir())
	_ = store.Write(record(configDir))

	for _, tt := range []struct{ name, path string }{
		{"the VM directory itself", configDir},
		{"a parent containing it", parent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &stateRunner{state: "stopped"}
			deps := createDeps{lima: lima.New(r), store: store}
			err := runMount(context.Background(), deps, "work", tt.path, "")
			if err == nil {
				t.Fatal("want an error: this path exposes the VM directory read/write")
			}
			if !strings.Contains(err.Error(), "/mnt/host/vm") {
				t.Errorf("error = %q, want it to point at the read-only service mount", err)
			}
			if len(r.ops) != 0 {
				t.Errorf("nothing must be touched, ops = %v", r.ops)
			}
			if got := specMounts(t, configDir); len(got) != 0 {
				t.Errorf("spec must be unchanged, got %+v", got)
			}
		})
	}
}

// TestMountUnusableRecord: a Record with no absolute VM directory must be
// rejected outright. filepath.Join("", "agent-vm.yaml") resolves against the
// process's working directory, so without this guard both commands would
// happily rewrite an unrelated agent-vm.yaml that happens to sit there.
func TestMountUnusableRecord(t *testing.T) {
	for _, tt := range []struct {
		name string
		rec  registry.Record
	}{
		{"empty config dir", registry.Record{Name: "work", Home: "/home/me.linux"}},
		{"relative config dir", registry.Record{Name: "work", Home: "/home/me.linux", ConfigDir: "vms/work"}},
		{"no guest home", registry.Record{Name: "work", ConfigDir: "/abs/vms/work"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, project, store := vmDir(t, "mounts:\n")
			_ = store.Write(tt.rec)
			deps := createDeps{lima: lima.New(&stateRunner{state: "stopped"}), store: store}

			err := runMount(context.Background(), deps, "work", project, "")
			if err == nil || !strings.Contains(err.Error(), "recreate the VM") {
				t.Errorf("runMount error = %v, want an unusable-record error", err)
			}
			err = runUnmount(context.Background(), deps, "work", project)
			if err == nil || !strings.Contains(err.Error(), "recreate the VM") {
				t.Errorf("runUnmount error = %v, want an unusable-record error", err)
			}
		})
	}
}

// TestMountCmdNormalizesVMName: a Record is always stored under the normalized
// name, so `avm mount My_Work` must reach the same VM `avm shell My_Work` does.
// Both commands are driven through RunE, which is where the normalization
// lives; each failure mode chosen here happens *after* the Record lookup, so
// "no VM named" would mean the lookup used the raw argument.
func TestMountCmdNormalizesVMName(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "agent-vm.yaml"), []byte("mounts:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := record(configDir)
	normalized, err := vmname.Normalize("My_Work")
	if err != nil {
		t.Fatal(err)
	}
	rec.Name = normalized
	if err := registry.NewStore(filepath.Join(root, "agent-vm")).Write(rec); err != nil {
		t.Fatal(err)
	}

	mount := newMountCmd()
	mount.SetContext(context.Background())
	err = mount.RunE(mount, []string{"My_Work", filepath.Join(t.TempDir(), "missing")})
	if err == nil || !strings.Contains(err.Error(), "mount source not found") {
		t.Errorf("avm mount error = %v, want it to get past the Record lookup", err)
	}

	unmount := newUnmountCmd()
	unmount.SetContext(context.Background())
	err = unmount.RunE(unmount, []string{"My_Work", "nope"})
	if err == nil || !strings.Contains(err.Error(), "is not mounted") {
		t.Errorf("avm unmount error = %v, want it to get past the Record lookup", err)
	}
}

func TestMountCmdSurface(t *testing.T) {
	mount := newMountCmd()
	if mount.Flags().Lookup("name") == nil {
		t.Error("avm mount must define --name")
	}
	for _, gone := range []string{"yes", "no-restart"} {
		if f := mount.Flags().Lookup(gone); f != nil {
			t.Errorf("avm mount must not define --%s", gone)
		}
	}
	if err := mount.Args(mount, []string{}); err == nil {
		t.Error("avm mount requires the VM name explicitly")
	}
	if err := mount.Args(mount, []string{"work", "/tmp/x", "extra"}); err == nil {
		t.Error("avm mount takes at most two arguments")
	}
	unmount := newUnmountCmd()
	if err := unmount.Args(unmount, []string{"work"}); err == nil {
		t.Error("avm unmount requires both a VM name and a target")
	}
}
