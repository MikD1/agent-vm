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

func TestValidateMounts(t *testing.T) {
	known := func(string) bool { return true }
	if err := (Spec{Mounts: []MountSpec{{Path: ""}}}).Validate(known); err == nil {
		t.Error("want error for empty mount path")
	}
	if err := (Spec{Mounts: []MountSpec{{Path: "../x", Name: "bad/name"}}}).Validate(known); err == nil {
		t.Error("want error for name with a slash")
	}
	if err := (Spec{Mounts: []MountSpec{{Path: "../x", Name: ".."}}}).Validate(known); err == nil {
		t.Error("want error for name '..'")
	}
	if err := (Spec{Mounts: []MountSpec{{Path: "../x"}, {Path: "../y", Name: "ok-1"}}}).Validate(known); err != nil {
		t.Errorf("unexpected error for valid mounts: %v", err)
	}
}

func TestResolveMounts(t *testing.T) {
	env := Env{
		ProjectName: "my-api", GuestUser: "me", GuestHome: "/home/me.linux",
		HostPath: "/Users/me/my-api",
		Mounts: []MountInput{
			{HostPath: "/Users/me/shared-lib"},                 // → ~/shared-lib
			{HostPath: "/Users/me/tools/cli", Name: "cli-x"},   // name override → ~/cli-x
			{HostPath: "/Users/me/shared-lib"},                 // duplicate host → deduped
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
