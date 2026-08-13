package provision

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
		"/etc/profile.d/mise.sh", // login shells
		"/etc/environment",       // non-login `avm shell`, via PAM
		"/etc/sudoers.d/",        // sudo ignores both of the above
		"visudo -cf",             // never install an unvalidated sudoers file
		"SHASUMS256.txt",         // the release binary is verified
		miseShims,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("mise.sh does not mention %q", want)
		}
	}
}

// miseAwkChecksumProgram extracts the awk program mise.sh uses to pick the
// matching SHASUMS256.txt line out of the script text itself, so this test
// exercises the real embedded program rather than a hand-copied stand-in that
// could silently drift from it.
func miseAwkChecksumProgram(t *testing.T, script string) string {
	t.Helper()
	re := regexp.MustCompile(`awk -v f="\$asset" '([^\n]*)' \\`)
	m := re.FindStringSubmatch(script)
	if m == nil {
		t.Fatal("mise.sh: could not locate the awk checksum-matching program")
	}
	return m[1]
}

// TestMiseScriptChecksumAwkToleratesDotSlashPrefix proves the awk program that
// picks the matching line out of SHASUMS256.txt handles the real release
// format: mise's published checksum file lists filenames "./"-prefixed (e.g.
// "./mise-v2026.8.4-linux-x64"), while the file actually downloaded to
// /var/tmp/${asset} has no such prefix. Before this fix, `$2 == f` never
// matched a "./"-prefixed line, sha256sum -c got zero input lines, exited
// non-zero, and `set -euo pipefail` aborted mise.sh on every VM — this test
// runs the awk program (extracted from the live script) against a realistic
// fixture and feeds its output straight into sha256sum -c to prove the whole
// pipeline, not just the awk match, now succeeds.
func TestMiseScriptChecksumAwkToleratesDotSlashPrefix(t *testing.T) {
	if _, err := exec.LookPath("awk"); err != nil {
		t.Skip("awk not available")
	}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		t.Skip("sha256sum not available")
	}
	b, err := guestScript("mise")
	if err != nil {
		t.Fatal(err)
	}
	program := miseAwkChecksumProgram(t, string(b))

	dir := t.TempDir()
	asset := "mise-v2026.8.4-linux-x64"
	assetPath := filepath.Join(dir, asset)
	if err := os.WriteFile(assetPath, []byte("fixture binary contents"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := exec.Command("sha256sum", assetPath).Output()
	if err != nil {
		t.Fatalf("sha256sum fixture: %v", err)
	}
	hash := strings.Fields(string(sum))[0]

	// A realistic SHASUMS256.txt: every filename "./"-prefixed, including a
	// sibling "-musl" asset whose name is a superset of the one we want, so a
	// substring match would be a false positive too.
	sums := strings.Join([]string{
		hash + "  ./" + asset,
		"1111111111111111111111111111111111111111111111111111111111111111  ./" + asset + "-musl",
		"2222222222222222222222222222222222222222222222222222222222222222  ./mise-v2026.8.4-linux-arm64",
		"",
	}, "\n")

	awkCmd := exec.Command("awk", "-v", "f="+asset, program)
	awkCmd.Stdin = strings.NewReader(sums)
	var awkOut bytes.Buffer
	awkCmd.Stdout = &awkOut
	var awkErr bytes.Buffer
	awkCmd.Stderr = &awkErr
	if err := awkCmd.Run(); err != nil {
		t.Fatalf("awk program failed on a ./-prefixed fixture: %v\nstderr: %s", err, awkErr.String())
	}
	wantLine := hash + "  " + asset
	if got := strings.TrimSpace(awkOut.String()); got != wantLine {
		t.Fatalf("awk output = %q, want %q", got, wantLine)
	}

	// Feed the awk output straight into sha256sum -c, exactly as mise.sh does,
	// with cwd set to where the fixture file actually lives.
	checkCmd := exec.Command("sha256sum", "-c", "-")
	checkCmd.Dir = dir
	checkCmd.Stdin = &awkOut
	var checkOut bytes.Buffer
	checkCmd.Stdout = &checkOut
	checkCmd.Stderr = &checkOut
	if err := checkCmd.Run(); err != nil {
		t.Fatalf("sha256sum -c failed on the awk output: %v\noutput: %s", err, checkOut.String())
	}
	if !strings.Contains(checkOut.String(), "OK") {
		t.Fatalf("sha256sum -c did not report OK: %s", checkOut.String())
	}
}
