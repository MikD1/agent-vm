package config

import "testing"

func strs(v ...string) *[]string { return &v }

func TestResolvePrecedence(t *testing.T) {
	env := Env{ProjectName: "my-api", GuestUser: "me", GuestHome: "/home/me.linux", SpecPresent: true}
	// flag modules override spec modules; flag cpus override spec cpus.
	flags := Flags{Modules: []string{"go"}, ModulesSet: true, CPUs: 8}
	spec := Spec{Modules: strs("node", "claude"), Resources: Resources{CPUs: 4, Memory: "8GiB"}}
	r, err := Resolve(flags, spec, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Modules) != 1 || r.Modules[0] != "go" {
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
	r2, _ := Resolve(Flags{}, Spec{Modules: strs()}, env)
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
	known := func(m string) bool { return m == "node" || m == "go" }
	bad := Spec{Modules: strs("node", "bogus")}
	if err := bad.Validate(known); err == nil {
		t.Error("want error for unknown module")
	}
	if err := (Spec{Resources: Resources{CPUs: 0, Memory: "16xb"}}).Validate(known); err == nil {
		t.Error("want error for bad memory")
	}
	if err := (Spec{Modules: strs("node"), Resources: Resources{Memory: "16GiB"}}).Validate(known); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
