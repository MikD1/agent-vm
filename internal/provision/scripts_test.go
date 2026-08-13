package provision

import (
	"strings"
	"testing"
)

func TestGuestScriptsPresent(t *testing.T) {
	for _, name := range []string{"system", "base", "docker", "mise"} {
		b, err := guestScript(name)
		if err != nil {
			t.Fatalf("guestScript(%q): %v", name, err)
		}
		if !strings.HasPrefix(string(b), "#!/usr/bin/env bash") {
			t.Errorf("%s.sh does not start with a bash shebang", name)
		}
		if !strings.Contains(string(b), "set -euo pipefail") {
			t.Errorf("%s.sh does not set -euo pipefail", name)
		}
	}
}

func TestGuestScriptUnknown(t *testing.T) {
	if _, err := guestScript("nope"); err == nil {
		t.Error("guestScript(\"nope\") = nil error, want an error")
	}
}

func TestMiseScriptWiresEveryShellContext(t *testing.T) {
	b, err := guestScript("mise")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"/etc/profile.d/mise.sh",   // login shells
		"/etc/environment",         // non-login `avm shell`, via PAM
		"/etc/sudoers.d/",          // sudo ignores both of the above
		"visudo -cf",               // never install an unvalidated sudoers file
		"SHASUMS256.txt",           // the release binary is verified
		miseShims,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("mise.sh does not mention %q", want)
		}
	}
}
