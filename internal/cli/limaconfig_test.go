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
		Workspace: config.Workspace{Mode: "mount", GuestPath: "/home/me/my-api", HostPath: "/h/my-api"},
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

func TestBuildLimaConfigCloneForwardsAgent(t *testing.T) {
	r := config.Resolved{
		Name: "my-api", User: "me",
		Resources: config.Resources{CPUs: 4, Memory: "4GiB", Disk: "120GiB"},
		Base:      config.Base{Image: "template:_images/ubuntu"},
		Workspace: config.Workspace{Mode: "clone", GuestPath: "/home/me/my-api", Repo: "git@h:a/b.git", Ref: "main"},
	}
	yamlOut, _ := buildLimaConfig(r, "/home/me")
	s := string(yamlOut)
	if !strings.Contains(s, "forwardAgent: true") {
		t.Errorf("clone mode must forward the SSH agent:\n%s", s)
	}
	if strings.Contains(s, "/h/my-api") {
		t.Error("clone mode must not add a host project mount")
	}
}
