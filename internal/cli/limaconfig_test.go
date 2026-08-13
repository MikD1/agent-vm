package cli

import (
	"strings"
	"testing"

	"github.com/MikD1/agent-vm/internal/config"
)

func TestBuildLimaConfigMount(t *testing.T) {
	r := config.Resolved{
		Name: "my-api", User: "me",
		Resources: config.Resources{CPUs: 8, Memory: "16GiB", Disk: "200GiB"},
		Base:      config.Base{Image: "corp-img"},
		Workspace: config.Mount{HostPath: "/h/my-api", GuestPath: "/home/me/my-api"},
	}
	yamlOut, err := buildLimaConfig(r, "/home/me")
	if err != nil {
		t.Fatal(err)
	}
	s := string(yamlOut)
	for _, want := range []string{"corp-img", "cpus: 8", "16GiB", "200GiB", "/h/my-api", "/home/me/my-api", "forwardAgent: false"} {
		if !strings.Contains(s, want) {
			t.Errorf("lima config missing %q:\n%s", want, s)
		}
	}
}

func TestBuildLimaConfigAdditionalMounts(t *testing.T) {
	r := config.Resolved{
		Name: "my-api", User: "me",
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		Workspace: config.Mount{HostPath: "/h/my-api", GuestPath: "/home/me/my-api"},
		Mounts: []config.Mount{
			{HostPath: "/h/shared-lib", GuestPath: "/home/me/shared-lib"},
			{HostPath: "/h/tools/cli", GuestPath: "/home/me/cli"},
		},
	}
	yamlOut, err := buildLimaConfig(r, "/home/me")
	if err != nil {
		t.Fatal(err)
	}
	s := string(yamlOut)
	for _, want := range []string{"/h/shared-lib", "/home/me/shared-lib", "/h/tools/cli", "/home/me/cli", "/h/my-api"} {
		if !strings.Contains(s, want) {
			t.Errorf("lima config missing %q:\n%s", want, s)
		}
	}
}
