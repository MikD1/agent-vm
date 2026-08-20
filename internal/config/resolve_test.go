package config

import (
	"strings"
	"testing"
)

func mods(v ...ModuleSpec) *[]ModuleSpec { return &v }

func TestResolvePrecedence(t *testing.T) {
	env := Env{ProjectName: "my-api", GuestUser: "me", GuestHome: "/home/me.linux"}
	// flag modules override spec modules; flag cpus override spec cpus.
	flags := Flags{Modules: []string{"go"}, ModulesSet: true, CPUs: 8}
	spec := Spec{Modules: mods(ModuleSpec{Name: "node"}, ModuleSpec{Name: "claude"}), Resources: Resources{CPUs: 4, Memory: "8GiB"}}
	r, err := Resolve(flags, spec, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Modules) != 1 || r.Modules[0].Name != "go" || r.Modules[0].Version != DefaultToolVersion {
		t.Errorf("modules = %v (flag should win)", r.Modules)
	}
	if r.Resources.CPUs != 8 {
		t.Errorf("cpus = %d (flag should win)", r.Resources.CPUs)
	}
	if r.Resources.Memory != "8GiB" {
		t.Errorf("memory = %q (spec should win over default)", r.Resources.Memory)
	}
	if r.Resources.Disk != DefaultDisk {
		t.Errorf("disk = %q (default should fill)", r.Resources.Disk)
	}
	if r.Base.Image != DefaultImage {
		t.Errorf("image = %q (default)", r.Base.Image)
	}
}

func TestResolveDefaultModulesOnlyWhenAbsent(t *testing.T) {
	env := Env{ProjectName: "p", GuestUser: "me", GuestHome: "/home/me.linux"}
	// No flag, no spec modules key → DefaultModules.
	r, _ := Resolve(Flags{}, Spec{}, env)
	if len(r.Modules) != len(DefaultModules) || r.Modules[0] != DefaultModules[0] {
		t.Errorf("modules = %v, want DefaultModules %v", r.Modules, DefaultModules)
	}
	// Explicit empty list → base only, NOT defaults.
	r2, _ := Resolve(Flags{}, Spec{Modules: mods()}, env)
	if len(r2.Modules) != 0 {
		t.Errorf("explicit empty modules should stay empty, got %v", r2.Modules)
	}
}

func TestValidate(t *testing.T) {
	if err := (Spec{Resources: Resources{CPUs: 0, Memory: "16xb"}}).Validate(); err == nil {
		t.Error("want error for bad memory")
	}
	if err := (Spec{Modules: mods(ModuleSpec{Name: "node"}), Resources: Resources{Memory: "16GiB"}}).Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateMounts(t *testing.T) {
	if err := (Spec{Mounts: []MountSpec{{Path: ""}}}).Validate(); err == nil {
		t.Error("want error for empty mount path")
	}
	if err := (Spec{Mounts: []MountSpec{{Path: "/x", Name: "bad/name"}}}).Validate(); err == nil {
		t.Error("want error for name with a slash")
	}
	if err := (Spec{Mounts: []MountSpec{{Path: "/x", Name: ".."}}}).Validate(); err == nil {
		t.Error("want error for name '..'")
	}
	if err := (Spec{Mounts: []MountSpec{{Path: "/x"}, {Path: "~/y", Name: "ok-1"}}}).Validate(); err != nil {
		t.Errorf("unexpected error for valid mounts: %v", err)
	}
}

// TestValidateMountPathsMustBeAbsolute pins decision 3: a relative path would
// make the VM's contents depend on where the config folder happens to sit.
func TestValidateMountPathsMustBeAbsolute(t *testing.T) {
	for _, bad := range []string{"../shared-lib", "lib", "./lib", "~x/y"} {
		if err := (Spec{Mounts: []MountSpec{{Path: bad}}}).Validate(); err == nil {
			t.Errorf("Validate(mount %q) = nil, want an error", bad)
		}
	}
	for _, ok := range []string{"/Users/me/projects/api", "~/projects/api"} {
		if err := (Spec{Mounts: []MountSpec{{Path: ok}}}).Validate(); err != nil {
			t.Errorf("Validate(mount %q) = %v, want nil", ok, err)
		}
	}
	// .. is rejected even inside an otherwise absolute path.
	if err := (Spec{Mounts: []MountSpec{{Path: "/Users/me/../etc"}}}).Validate(); err == nil {
		t.Error("want error for .. inside a mount path")
	}
}

// TestValidateName covers the optional `name:` key. Full normalization lives in
// the cli layer (vmname); config only rejects shapes that can never be a name.
func TestValidateName(t *testing.T) {
	if err := (Spec{Name: "work"}).Validate(); err != nil {
		t.Errorf("Validate(name work) = %v, want nil", err)
	}
	if err := (Spec{}).Validate(); err != nil {
		t.Errorf("an absent name must stay valid: %v", err)
	}
	for _, bad := range []string{"has/slash", ".", "..", "has space"} {
		if err := (Spec{Name: bad}).Validate(); err == nil {
			t.Errorf("Validate(name %q) = nil, want an error", bad)
		}
	}
}

func TestResolveMounts(t *testing.T) {
	env := Env{
		ProjectName: "work", GuestUser: "me", GuestHome: "/home/me.linux",
		ConfigDir: "/Users/me/vms/work",
		Mounts: []MountInput{
			{HostPath: "/Users/me/shared-lib"},               // → ~/shared-lib
			{HostPath: "/Users/me/tools/cli", Name: "cli-x"}, // name override → ~/cli-x
			{HostPath: "/Users/me/shared-lib"},               // duplicate host → deduped
		},
	}
	r, err := Resolve(Flags{}, Spec{}, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Mounts) != 2 {
		t.Fatalf("want 2 mounts after dedupe, got %d (%+v)", len(r.Mounts), r.Mounts)
	}
	if r.Mounts[0].HostPath != "/Users/me/shared-lib" || r.Mounts[0].GuestPath != "/home/me.linux/shared-lib" {
		t.Errorf("mount[0] = %+v", r.Mounts[0])
	}
	if r.Mounts[1].GuestPath != "/home/me.linux/cli-x" {
		t.Errorf("mount[1] guest = %q (name override)", r.Mounts[1].GuestPath)
	}
}

func TestResolveMountCollision(t *testing.T) {
	env := Env{
		ProjectName: "work", GuestUser: "me", GuestHome: "/home/me.linux",
		Mounts: []MountInput{{HostPath: "/a/shared"}, {HostPath: "/b/shared"}}, // both → ~/shared
	}
	_, err := Resolve(Flags{}, Spec{}, env)
	if err == nil {
		t.Fatal("want collision error for two mounts mapping to the same guest path")
	}
	if !strings.Contains(err.Error(), "pass a different name") {
		t.Errorf("collision error = %q, want it to suggest a different name", err)
	}
}

// TestResolveNoMounts pins acceptance criterion 4: a VM with no mounts at all is
// a valid, meaningful state — a domain you have not attached projects to yet.
func TestResolveNoMounts(t *testing.T) {
	r, err := Resolve(Flags{}, Spec{}, Env{
		ProjectName: "work", GuestUser: "me", GuestHome: "/home/me.linux",
		ConfigDir: "/Users/me/vms/work",
	})
	if err != nil {
		t.Fatalf("a VM with no mounts must resolve: %v", err)
	}
	if len(r.Mounts) != 0 {
		t.Errorf("Mounts = %+v, want empty", r.Mounts)
	}
}

// TestResolveCarriesConfigDirAndHome: both are create-time facts the Record
// stores, and `avm mount` later needs Home to build a new mount's guest path.
func TestResolveCarriesConfigDirAndHome(t *testing.T) {
	r, err := Resolve(Flags{}, Spec{}, Env{
		ProjectName: "work", GuestUser: "me", GuestHome: "/home/me.linux",
		ConfigDir: "/Users/me/vms/work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.ConfigDir != "/Users/me/vms/work" {
		t.Errorf("ConfigDir = %q", r.ConfigDir)
	}
	if r.Home != "/home/me.linux" {
		t.Errorf("Home = %q", r.Home)
	}
}

func TestValidateModules(t *testing.T) {
	ok := []Spec{
		{Modules: &[]ModuleSpec{{Name: "node", Version: "lts"}}},
		{Modules: &[]ModuleSpec{{Name: "npm:@openai/codex"}}},
		{Modules: &[]ModuleSpec{{Name: "aqua:owner/repo", Version: "1.2.3"}}},
	}
	for _, s := range ok {
		if err := s.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v, want nil", *s.Modules, err)
		}
	}
	bad := []Spec{
		{Modules: &[]ModuleSpec{{Name: ""}}},
		{Modules: &[]ModuleSpec{{Name: "no spaces"}}},
		{Modules: &[]ModuleSpec{{Name: "node", Version: "1 2"}}},
		{Modules: &[]ModuleSpec{{Name: "node"}, {Name: "node", Version: "22"}}},
	}
	for _, s := range bad {
		if err := s.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error", *s.Modules)
		}
	}
}

func TestResolveFillsDefaultVersion(t *testing.T) {
	spec := Spec{Modules: &[]ModuleSpec{{Name: "node"}, {Name: "go", Version: "1.24"}}}
	r, err := Resolve(Flags{}, spec, Env{ProjectName: "p", GuestUser: "u", GuestHome: "/home/u"})
	if err != nil {
		t.Fatal(err)
	}
	want := []ModuleSpec{{Name: "node", Version: DefaultToolVersion}, {Name: "go", Version: "1.24"}}
	if len(r.Modules) != 2 || r.Modules[0] != want[0] || r.Modules[1] != want[1] {
		t.Errorf("Modules = %+v, want %+v", r.Modules, want)
	}
}

func TestValidateFiles(t *testing.T) {
	ok := Spec{Files: map[string]FileSpec{
		"a.json": {To: "~/.a/a.json"},
		"b.json": {To: "/etc/b.json", Mode: "0600"},
	}}
	if err := ok.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
	bad := map[string]Spec{
		"empty source":   {Files: map[string]FileSpec{"": {To: "~/x"}}},
		"empty dest":     {Files: map[string]FileSpec{"a": {To: ""}}},
		"relative dest":  {Files: map[string]FileSpec{"a": {To: "x/y"}}},
		"dotdot in dest": {Files: map[string]FileSpec{"a": {To: "~/../x"}}},
		"bad mode":       {Files: map[string]FileSpec{"a": {To: "~/x", Mode: "644"}}},
		"non octal mode": {Files: map[string]FileSpec{"a": {To: "~/x", Mode: "0abc"}}},
		"duplicate dest": {Files: map[string]FileSpec{"a": {To: "~/x"}, "b": {To: "~/x"}}},
	}
	for name, s := range bad {
		if err := s.Validate(); err == nil {
			t.Errorf("Validate(%s) = nil, want an error", name)
		}
	}
}

func TestResolveFiles(t *testing.T) {
	env := Env{
		ProjectName: "p", GuestUser: "u", GuestHome: "/home/u.linux",
		Files: []FileInput{
			{Rel: "codex-auth.json", To: "~/.codex/auth.json", Mode: "0600"},
			{Rel: "claude-settings.json", To: "~/.claude/settings.json"},
		},
	}
	r, err := Resolve(Flags{}, Spec{}, env)
	if err != nil {
		t.Fatal(err)
	}
	// Sorted by destination, and ~/ expanded against the guest home.
	want := []FileCopy{
		{Rel: "claude-settings.json", To: "/home/u.linux/.claude/settings.json", Mode: DefaultFileMode},
		{Rel: "codex-auth.json", To: "/home/u.linux/.codex/auth.json", Mode: "0600"},
	}
	if len(r.Files) != 2 || r.Files[0] != want[0] || r.Files[1] != want[1] {
		t.Errorf("Files = %+v, want %+v", r.Files, want)
	}
}

func TestValidateScripts(t *testing.T) {
	if err := (Spec{Scripts: []string{"provision/docker.sh"}}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
	for name, s := range map[string]Spec{
		"empty":  {Scripts: []string{""}},
		"dotdot": {Scripts: []string{"../evil.sh"}},
	} {
		if err := s.Validate(); err == nil {
			t.Errorf("Validate(%s) = nil, want an error", name)
		}
	}
}

func TestResolvePreservesScriptOrder(t *testing.T) {
	env := Env{
		ProjectName: "p", GuestUser: "u", GuestHome: "/home/u",
		Scripts: []string{"/h/p/b.sh", "/h/p/a.sh"},
	}
	r, err := Resolve(Flags{}, Spec{}, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Scripts) != 2 || r.Scripts[0] != "/h/p/b.sh" || r.Scripts[1] != "/h/p/a.sh" {
		t.Errorf("Scripts = %v, want spec order preserved", r.Scripts)
	}
}

func TestResolveModulesFromFlags(t *testing.T) {
	f := Flags{Modules: []string{"node@lts", "go"}, ModulesSet: true}
	r, err := Resolve(f, Spec{}, Env{ProjectName: "p", GuestUser: "u", GuestHome: "/home/u"})
	if err != nil {
		t.Fatal(err)
	}
	want := []ModuleSpec{{Name: "node", Version: "lts"}, {Name: "go", Version: DefaultToolVersion}}
	if len(r.Modules) != 2 || r.Modules[0] != want[0] || r.Modules[1] != want[1] {
		t.Errorf("Modules = %+v, want %+v", r.Modules, want)
	}
}
