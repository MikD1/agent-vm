package provision

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

// TestMiseVersionCoversAquaClaudeFix guards the pin against being lowered back
// into the range where `claude` cannot install. mise resolves the module to the
// aqua package anthropics/claude-code, which serves current versions through a
// version_override that switches the package type to github_release. Releases
// before v2026.8.5 dropped that explicit type, kept the parent http package's
// empty url and aborted phase 3 with "builder error: relative URL without a
// base"; v2026.8.9 added the glibc-over-musl asset preference the same package
// relies on. Anything older breaks every VM whose spec lists claude.
func TestMiseVersionCoversAquaClaudeFix(t *testing.T) {
	const minYear, minMonth, minPatch = 2026, 8, 9

	m := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`).FindStringSubmatch(miseVersion)
	if m == nil {
		t.Fatalf("miseVersion = %q, want a vYYYY.M.P release tag", miseVersion)
	}
	var got [3]int
	for i := range got {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			t.Fatalf("miseVersion = %q: %v", miseVersion, err)
		}
		got[i] = n
	}
	want := [3]int{minYear, minMonth, minPatch}
	for i := range got {
		if got[i] > want[i] {
			return
		}
		if got[i] < want[i] {
			t.Fatalf("miseVersion = %q, want v%d.%d.%d or newer (older mise cannot install the claude module)",
				miseVersion, minYear, minMonth, minPatch)
		}
	}
}
