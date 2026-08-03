package cli

import (
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
	"github.com/MikD1/agent-vm/internal/provision"
	"gopkg.in/yaml.v3"
)

// parseMounts pulls the mounts list back out of the rendered Lima YAML.
func parseMounts(t *testing.T, out []byte) []map[string]any {
	t.Helper()
	var doc struct {
		Mounts []map[string]any `yaml:"mounts"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("rendered config is not valid YAML: %v", err)
	}
	return doc.Mounts
}

func TestBuildLimaConfigBasics(t *testing.T) {
	r := config.Resolved{
		Name: "work", User: "me",
		Resources: config.Resources{CPUs: 8, Memory: "16GiB", Disk: "200GiB"},
		Base:      config.Base{Image: "corp-img"},
		ConfigDir: "/Users/me/vms/work",
		Home:      "/home/me",
	}
	out, err := buildLimaConfig(r, "/home/me", hostMacOS)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"corp-img", "cpus: 8", "16GiB", "200GiB", "forwardAgent: false"} {
		if !strings.Contains(s, want) {
			t.Errorf("lima config missing %q:\n%s", want, s)
		}
	}
}

// TestLimaMountsServiceMountIsFirstAndReadOnly pins the one service mount: the
// VM directory, read-only, ahead of every project.
func TestLimaMountsServiceMountIsFirstAndReadOnly(t *testing.T) {
	got := limaMounts("/Users/me/vms/work", []config.Mount{
		{HostPath: "/h/api", GuestPath: "/home/me/api"},
	})
	if len(got) != 2 {
		t.Fatalf("want 2 mounts (service + 1 project), got %d: %+v", len(got), got)
	}
	svc, ok := got[0].(map[string]any)
	if !ok {
		t.Fatalf("mount[0] is not a mapping: %T", got[0])
	}
	if svc["location"] != "/Users/me/vms/work" {
		t.Errorf("service mount location = %v, want the VM directory", svc["location"])
	}
	if svc["mountPoint"] != provision.GuestConfigMount {
		t.Errorf("service mount point = %v, want %s", svc["mountPoint"], provision.GuestConfigMount)
	}
	if svc["writable"] != false {
		t.Errorf("service mount must be read-only, got writable=%v", svc["writable"])
	}
	proj := got[1].(map[string]any)
	if proj["location"] != "/h/api" || proj["writable"] != true {
		t.Errorf("project mount = %+v, want /h/api writable", proj)
	}
}

func TestLimaMountsWithNoProjects(t *testing.T) {
	got := limaMounts("/Users/me/vms/work", nil)
	if len(got) != 1 {
		t.Fatalf("a VM with no projects still gets the service mount, got %+v", got)
	}
}

func TestBuildLimaConfigRendersAllMounts(t *testing.T) {
	r := config.Resolved{
		Name: "work", User: "me",
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		ConfigDir: "/Users/me/vms/work",
		Home:      "/home/me",
		Mounts: []config.Mount{
			{HostPath: "/h/shared-lib", GuestPath: "/home/me/shared-lib"},
			{HostPath: "/h/tools/cli", GuestPath: "/home/me/cli"},
		},
	}
	out, err := buildLimaConfig(r, "/home/me", hostMacOS)
	if err != nil {
		t.Fatal(err)
	}
	mounts := parseMounts(t, out)
	if len(mounts) != 3 {
		t.Fatalf("want service + 2 projects, got %d: %+v", len(mounts), mounts)
	}
	if mounts[0]["mountPoint"] != provision.GuestConfigMount {
		t.Errorf("service mount is not first: %+v", mounts)
	}
	for _, want := range []string{"/h/shared-lib", "/home/me/shared-lib", "/h/tools/cli", "/home/me/cli"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("lima config missing %q:\n%s", want, out)
		}
	}
}

func TestBuildLimaConfigLinux(t *testing.T) {
	r := config.Resolved{
		Name: "my-api", User: "me",
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		ConfigDir: "/h/config",
		Mounts: []config.Mount{{HostPath: "/h/my-api", GuestPath: "/home/me/my-api"}},
	}
	yamlOut, err := buildLimaConfig(r, "/home/me", hostLinux)
	if err != nil {
		t.Fatal(err)
	}
	s := string(yamlOut)
	for _, want := range []string{"vmType: qemu", "mountType: 9p", "/h/my-api"} {
		if !strings.Contains(s, want) {
			t.Errorf("Linux Lima config missing %q:\n%s", want, s)
		}
	}
}
