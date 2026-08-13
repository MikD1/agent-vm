package config

import "testing"

func mods(v ...ModuleSpec) *[]ModuleSpec { return &v }

func TestResolvePrecedence(t *testing.T) {
	env := Env{ProjectName: "my-api", GuestUser: "me", GuestHome: "/home/me.linux", SpecPresent: true}
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

func TestResolveMountVsClone(t *testing.T) {
	env := Env{ProjectName: "my-api", GuestUser: "me", GuestHome: "/home/me.linux", HostPath: "/Users/me/my-api"}
	mount, _ := Resolve(Flags{}, Spec{}, env)
	if mount.Workspace.Mode != "mount" || mount.Workspace.HostPath != "/Users/me/my-api" {
		t.Errorf("mount workspace = %+v", mount.Workspace)
	}
	if mount.Workspace.GuestPath != "/home/me.linux/my-api" {
		t.Errorf("guestPath = %q", mount.Workspace.GuestPath)
	}
	clone, _ := Resolve(Flags{Repo: "git@h:acme/my-api.git", Ref: "main"}, Spec{}, env)
	if clone.Workspace.Mode != "clone" || clone.Workspace.Repo == "" || clone.Workspace.Ref != "main" {
		t.Errorf("clone workspace = %+v", clone.Workspace)
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
	if err := (Spec{Mounts: []MountSpec{{Path: "../x", Name: "bad/name"}}}).Validate(); err == nil {
		t.Error("want error for name with a slash")
	}
	if err := (Spec{Mounts: []MountSpec{{Path: "../x", Name: ".."}}}).Validate(); err == nil {
		t.Error("want error for name '..'")
	}
	if err := (Spec{Mounts: []MountSpec{{Path: "../x"}, {Path: "../y", Name: "ok-1"}}}).Validate(); err != nil {
		t.Errorf("unexpected error for valid mounts: %v", err)
	}
}

func TestResolveMounts(t *testing.T) {
	env := Env{
		ProjectName: "my-api", GuestUser: "me", GuestHome: "/home/me.linux",
		HostPath: "/Users/me/my-api",
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

func TestResolveMountCollisionWithSibling(t *testing.T) {
	env := Env{
		ProjectName: "my-api", GuestUser: "me", GuestHome: "/home/me.linux", HostPath: "/Users/me/my-api",
		Mounts: []MountInput{{HostPath: "/a/shared"}, {HostPath: "/b/shared"}}, // both → ~/shared
	}
	if _, err := Resolve(Flags{}, Spec{}, env); err == nil {
		t.Error("want collision error for two mounts mapping to the same guest path")
	}
}

func TestResolveMountCollisionWithPrimary(t *testing.T) {
	env := Env{
		ProjectName: "my-api", GuestUser: "me", GuestHome: "/home/me.linux", HostPath: "/Users/me/my-api",
		Mounts: []MountInput{{HostPath: "/elsewhere/my-api"}}, // basename my-api → clashes with primary
	}
	if _, err := Resolve(Flags{}, Spec{}, env); err == nil {
		t.Error("want collision error against the primary workspace")
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
			{Root: RootSecrets, Rel: "codex-auth.json", To: "~/.codex/auth.json", Mode: "0600"},
			{Root: RootWorkspace, Rel: "claude-settings.json", To: "~/.claude/settings.json"},
		},
	}
	r, err := Resolve(Flags{}, Spec{}, env)
	if err != nil {
		t.Fatal(err)
	}
	// Sorted by destination, and ~/ expanded against the guest home.
	want := []FileCopy{
		{Root: RootWorkspace, Rel: "claude-settings.json", To: "/home/u.linux/.claude/settings.json", Mode: DefaultFileMode},
		{Root: RootSecrets, Rel: "codex-auth.json", To: "/home/u.linux/.codex/auth.json", Mode: "0600"},
	}
	if len(r.Files) != 2 || r.Files[0] != want[0] || r.Files[1] != want[1] {
		t.Errorf("Files = %+v, want %+v", r.Files, want)
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
