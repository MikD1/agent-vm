package cli

import (
	"os"
	"testing"
)

func TestDeriveGuestUser(t *testing.T) {
	cases := map[string]string{
		"Alice":       "alice",
		"m.doshevsky": "m_doshevsky",
		"9bad":        "_9bad",
		"-leading":    "_-leading",
		"":            "_",
	}
	for in, want := range cases {
		if got := deriveGuestUser(in); got != want {
			t.Errorf("deriveGuestUser(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGuestHomeFromInfo(t *testing.T) {
	info := []byte(`{"defaultTemplate":{"user":{"name":"user.linux","home":"/home/user.linux"}}}`)
	home, err := guestHome(info, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if home != "/home/alice" {
		t.Errorf("guestHome = %q, want /home/alice", home)
	}
}

func TestGuestHomeFallback(t *testing.T) {
	home, err := guestHome([]byte(`{}`), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if home != "/home/alice.linux" {
		t.Errorf("fallback home = %q", home)
	}
}

func TestResolveTargetName(t *testing.T) {
	// explicit arg is normalized
	n, err := resolveTargetName("My_API", "/nonexistent")
	if err != nil || n != "my-api" {
		t.Fatalf("explicit = %q, %v", n, err)
	}
	// from cwd basename when a spec exists
	dir := t.TempDir()
	specDir := dir + "/Cool_Proj"
	_ = osMkdirAll(specDir)
	_ = osWriteFile(specDir+"/.agent-vm.yaml", "modules: []\n")
	n2, err := resolveTargetName("", specDir)
	if err != nil || n2 != "cool-proj" {
		t.Fatalf("cwd = %q, %v", n2, err)
	}
	// error when no arg and no spec
	if _, err := resolveTargetName("", dir); err == nil {
		t.Error("want error with no arg and no spec")
	}
}

func TestGuestHomeRealistic(t *testing.T) {
	// Realistic Lima output: name is a plain username, home has .linux suffix.
	info := []byte(`{"defaultTemplate":{"user":{"name":"alice","home":"/home/alice.linux"}}}`)
	home, err := guestHome(info, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if home != "/home/bob.linux" {
		t.Errorf("guestHome = %q, want /home/bob.linux", home)
	}
}

func osMkdirAll(p string) error        { return os.MkdirAll(p, 0o755) }
func osWriteFile(p, s string) error    { return os.WriteFile(p, []byte(s), 0o644) }
