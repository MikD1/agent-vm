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

// TestMiseScriptBoundsItsDownloads guards the fix for a Phase 2 stall that
// looked exactly like a hang: mise's ~95 MB release binary comes from GitHub's
// asset CDN, and the bare `curl -fsSL` that fetched it had no timeout, no retry
// and no output. On a network path that blocks or throttles that CDN, curl waited
// forever while lima.Provision buffered the guest's stdout, so `avm create`
// printed "==> Phase 2: mise" and then nothing — no progress, no error, no way to
// tell a slow download from a dead one. Every download in the script must stay
// bounded, and the script must say what it is doing on stderr, the stream that
// reaches the terminal while the phase runs.
func TestMiseScriptBoundsItsDownloads(t *testing.T) {
	b, err := guestScript("mise")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"--connect-timeout", // never block on an unreachable host
		"--speed-limit",     // abort a transfer that stalls mid-flight
		"--speed-time",      //   (both halves of the stall guard)
		"--retry",           // a blip must not fail the whole phase
		"--continue-at",     // a retry costs the remainder, not the whole file
		"mise: downloading", // stdout is buffered, so say it on stderr
		">&2",               //   (the stream avm forwards live)
	} {
		if !strings.Contains(s, want) {
			t.Errorf("mise.sh does not mention %q", want)
		}
	}
	// A silent, unbounded curl is what caused the stall — no download may go back
	// to one. Every curl call must run through the bounded fetch helper.
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "curl ") && !strings.Contains(line, "curl -fL \\") {
			t.Errorf("mise.sh calls curl directly (%q); use the bounded fetch helper", line)
		}
	}
}

// TestMiseScriptInstallsFromTheCompressedArchive pins the choice of release
// asset. mise publishes the same binary four ways, and the bare one is by far
// the largest: ~95 MB against ~22 MB for .tar.xz and ~37 MB for .tar.gz. Phase 2
// pulled the bare binary, so on a throttled path to GitHub's asset CDN it spent
// twenty minutes moving bytes with nothing on screen — the compressed archive is
// the same install for a quarter of the transfer. xz is preferred, gzip is the
// fallback for an image without it, and the binary is unpacked from mise/bin/mise
// rather than installed directly.
func TestMiseScriptInstallsFromTheCompressedArchive(t *testing.T) {
	b, err := guestScript("mise")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"ext=tar.xz",                   // the small asset by default
		"ext=tar.gz",                   //   with a fallback that needs no xz
		"command -v xz",                // which of the two is decided at runtime
		"tar -xf",                      // both are archives now, not a raw binary
		"/var/tmp/mise/bin/mise",       // the binary inside the archive
		"failed checksum verification", // a bad resume must not persist
	} {
		if !strings.Contains(s, want) {
			t.Errorf("mise.sh does not mention %q", want)
		}
	}
	// The checksum is verified against the file that is actually installed, so
	// the asset name the digest is matched on must be the archive.
	if strings.Contains(s, `asset="mise-${MISE_VERSION}-linux-${arch}"`) {
		t.Error("mise.sh still downloads the bare (~95 MB) release binary")
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

// systemShellFunc pulls one shell function out of system.sh by name, so a test
// exercises the function the guest actually runs rather than a copy that can
// drift from it — the same technique miseAwkChecksumProgram uses for the awk
// program in mise.sh.
func systemShellFunc(t *testing.T, script, name string) string {
	t.Helper()
	start := strings.Index(script, name+"() {")
	if start < 0 {
		t.Fatalf("system.sh: no shell function named %q", name)
	}
	end := strings.Index(script[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("system.sh: function %q is not closed at column 0", name)
	}
	return script[start : start+end+len("\n}\n")]
}

// TestSystemScriptReadsEveryCertificateEncoding guards the fix for the failure
// this phase was built to prevent and then walked straight into: a corporate CA
// that avm never installed, followed by "invalid peer certificate:
// UnknownIssuer" from mise three phases later, with the VM already rolled back.
//
// The phase used to glob *.pem only. A corporate root CA is handed out as a
// .crt or .cer far more often than as a .pem, and just as often DER-encoded
// rather than PEM — so the single most likely file a user drops into
// ca-certificates/ was skipped in silence. PEM exported from a browser or moved
// through Windows tooling also arrives with CRLF endings and no final newline,
// which used to concatenate -----END----- and the next -----BEGIN----- onto one
// line and make every certificate after the first unparseable.
//
// This runs the real avm_to_pem out of the embedded script against each of those
// shapes and feeds the result to OpenSSL, which is the consumer that matters:
// update-ca-certificates rejects anything it cannot parse.
func TestSystemScriptReadsEveryCertificateEncoding(t *testing.T) {
	for _, bin := range []string{"bash", "openssl", "awk"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available", bin)
		}
	}
	b, err := guestScript("system")
	if err != nil {
		t.Fatal(err)
	}
	fn := systemShellFunc(t, string(b), "avm_to_pem")

	dir := t.TempDir()
	// Two self-signed certificates, so the chain case has something to chain.
	var pems []string
	for i, cn := range []string{"AVM Test Root", "AVM Test Sub"} {
		out := filepath.Join(dir, "gen"+strconv.Itoa(i)+".pem")
		cmd := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
			"-keyout", filepath.Join(dir, "key"+strconv.Itoa(i)+".pem"),
			"-out", out, "-days", "1", "-subj", "/CN="+cn)
		if o, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("openssl could not generate a fixture certificate: %v\n%s", err, o)
		}
		body, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		pems = append(pems, string(body))
	}

	der := filepath.Join(dir, "corp-root.der")
	if o, err := exec.Command("openssl", "x509", "-in", filepath.Join(dir, "gen0.pem"),
		"-outform", "der", "-out", der).CombinedOutput(); err != nil {
		t.Fatalf("openssl der conversion: %v\n%s", err, o)
	}
	derBody, err := os.ReadFile(der)
	if err != nil {
		t.Fatal(err)
	}

	// A PEM as Windows tooling hands it over: CRLF endings, no final newline.
	crlf := strings.ReplaceAll(strings.TrimRight(pems[0], "\n"), "\n", "\r\n")

	cases := []struct {
		name  string
		file  string
		body  []byte
		certs int
	}{
		{"pem", "corp.pem", []byte(pems[0]), 1},
		{"der named .crt", "corp.crt", derBody, 1},
		{"chain", "corp-chain.pem", []byte(pems[0] + pems[1]), 2},
		{"crlf without trailing newline", "corp-win.crt", []byte(crlf), 1},
		{"not a certificate", "notes.txt", []byte("just some text\n"), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := filepath.Join(dir, tc.file)
			if err := os.WriteFile(src, tc.body, 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", "-c", fn+"\navm_to_pem \"$1\"\n", "--", src)
			var out, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errb
			if err := cmd.Run(); err != nil {
				t.Fatalf("avm_to_pem: %v\nstderr: %s", err, errb.String())
			}
			got := out.String()
			if tc.certs == 0 {
				if got != "" {
					t.Fatalf("avm_to_pem returned %q for a non-certificate, want nothing", got)
				}
				return
			}
			if n := strings.Count(got, "-----BEGIN CERTIFICATE-----"); n != tc.certs {
				t.Fatalf("avm_to_pem emitted %d certificate(s), want %d:\n%s", n, tc.certs, got)
			}
			if strings.Contains(got, "\r") {
				t.Error("avm_to_pem kept CR characters; the bundle must be LF-only")
			}
			if !strings.HasSuffix(got, "\n") {
				t.Error("avm_to_pem output has no trailing newline; concatenation would corrupt the next certificate")
			}
			// The consumer's own parser is the only verdict that counts.
			check := exec.Command("openssl", "storeutl", "-noout", "-certs", "/dev/stdin")
			check.Stdin = strings.NewReader(got)
			if o, err := check.CombinedOutput(); err != nil {
				t.Fatalf("openssl could not parse avm_to_pem output: %v\n%s\n%s", err, o, got)
			} else if want := "Total found: " + strconv.Itoa(tc.certs); !strings.Contains(string(o), want) {
				t.Errorf("openssl reported %q, want %q", strings.TrimSpace(string(o)), want)
			}
		})
	}
}

// TestSystemScriptExtendsTheSystemStore pins the difference between adding trust
// and replacing it. The phase used to export SSL_CERT_FILE (and CURL_CA_BUNDLE,
// REQUESTS_CA_BUNDLE, GIT_SSL_CAINFO) pointing at a bundle holding the host CAs
// and nothing else — those variables replace the root list rather than extend
// it, so every VM built with a corporate CA lost the public roots. That is
// invisible while a proxy re-signs everything and fails the moment one host is
// on its inspection-bypass list, with the same UnknownIssuer a missing CA gives.
// NODE_EXTRA_CA_CERTS is the exception and must keep the host-only bundle: node
// adds it to its built-in roots.
func TestSystemScriptExtendsTheSystemStore(t *testing.T) {
	b, err := guestScript("system")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`SYSTEM_BUNDLE=/etc/ssl/certs/ca-certificates.crt`, // public roots + ours
		`TRUST_BUNDLE="$SYSTEM_BUNDLE"`,
		`export SSL_CERT_FILE="$TRUST_BUNDLE"`,
		`export CURL_CA_BUNDLE="$TRUST_BUNDLE"`,
		`export REQUESTS_CA_BUNDLE="$TRUST_BUNDLE"`,
		`export GIT_SSL_CAINFO="$TRUST_BUNDLE"`,
		`export SSL_CERT_DIR="/etc/ssl/certs"`,    // rustls-native-certs reads this too
		`export NODE_EXTRA_CA_CERTS="$CA_BUNDLE"`, // additive: host CAs only
		`update-ca-certificates`,                  // the merged store is rebuilt first
	} {
		if !strings.Contains(s, want) {
			t.Errorf("system.sh does not contain %q", want)
		}
	}
	if strings.Contains(s, `export SSL_CERT_FILE="$CA_BUNDLE"`) {
		t.Error("system.sh still replaces the root list with the host CAs alone")
	}
}

// TestSystemScriptReportsWhatItDid keeps the phase from being silent again. avm
// buffers the guest's stdout and streams only stderr, so anything this phase does
// not say on stderr is invisible while it runs — and a certificate step that says
// nothing is indistinguishable from one that found nothing, which is exactly how
// a skipped corporate CA turned into an unexplained TLS failure in phase 3.
func TestSystemScriptReportsWhatItDid(t *testing.T) {
	b, err := guestScript("system")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"system: trusting",                // one line per certificate, with its subject
		"CA certificate(s) installed",     // the count, so zero is legible
		"no custom CA certificates",       // the empty case says so out loud
		"is not a PEM or DER certificate", // a file that is not a certificate is named
		"cannot verify TLS to",            // the preflight that predicts the phase 3 failure
		"avm recreate",                    // every message ends in something to do
	} {
		if !strings.Contains(s, want) {
			t.Errorf("system.sh never says %q", want)
		}
	}
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "echo \"system:") && !strings.HasSuffix(trimmed, ">&2") {
			t.Errorf("system.sh reports on stdout, which avm buffers: %q", trimmed)
		}
	}
}
