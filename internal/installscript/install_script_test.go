// Package installscript tests the repository-level install.sh script.
package installscript

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallScriptInstallsLimaWithHomebrewAndAvm(t *testing.T) {
	root := repoRoot(t)
	env := newScriptEnv(t, "v1.2.3", "darwin", "arm64")
	env.writeFakeUname("Darwin", "arm64")
	env.writeFakeCurl(curlServe, curlServe)
	env.writeFakeBrew()

	out, err := env.run(root)
	if err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, out)
	}

	gotBrew := strings.TrimSpace(readFile(t, env.brewLog))
	if gotBrew != "install lima" {
		t.Fatalf("brew args = %q, want %q", gotBrew, "install lima")
	}

	installed := filepath.Join(env.installDir, "avm")
	got := readFile(t, installed)
	if got != env.binaryContent {
		t.Fatalf("installed avm content = %q, want %q", got, env.binaryContent)
	}
	assertExecutable(t, installed)
}

func TestInstallScriptUsesPinnedVersionWithoutLatestLookup(t *testing.T) {
	root := repoRoot(t)
	env := newScriptEnv(t, "v9.8.7", "darwin", "amd64")
	env.writeFakeUname("Darwin", "x86_64")
	env.writeFakeCurl(curlFail, curlFail)
	env.writeFakeLimactl()
	env.extraEnv = append(env.extraEnv, "AVM_VERSION=v9.8.7")

	out, err := env.run(root)
	if err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, out)
	}

	curlLog := readFile(t, env.curlLog)
	if strings.Contains(curlLog, "releases/latest") {
		t.Fatalf("release discovery ran for a pinned install:\n%s", curlLog)
	}

	installed := filepath.Join(env.installDir, "avm")
	if got := readFile(t, installed); got != env.binaryContent {
		t.Fatalf("installed avm content = %q, want %q", got, env.binaryContent)
	}
}

// TestInstallScriptResolvesLatestWithoutTheGitHubAPI covers the network this
// installer actually has to survive: github.com passes, api.github.com answers
// 403. That is a corporate proxy blocking the API host, or GitHub rate-limiting
// a shared egress IP — in both cases the release is public and its assets
// download fine one host over, but the installer used to ask only the API and
// die with "failed to resolve latest avm release". The version is discoverable
// without any API call: /releases/latest redirects to /releases/tag/<version>.
func TestInstallScriptResolvesLatestWithoutTheGitHubAPI(t *testing.T) {
	root := repoRoot(t)
	env := newScriptEnv(t, "v1.2.3", "arm64")
	env.writeFakeUname("Darwin", "arm64")
	env.writeFakeCurl(curlServe, curlFail)
	env.writeFakeLimactl()

	out, err := env.run(root)
	if err != nil {
		t.Fatalf("install.sh failed with a blocked API: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(env.installDir, "avm")); got != env.binaryContent {
		t.Fatalf("installed avm content = %q, want %q", got, env.binaryContent)
	}
	// The redirect answers the question outright, so the API is never worth a
	// request — on a network that blocks it, every such request is a stall.
	if curlLog := readFile(t, env.curlLog); strings.Contains(curlLog, "api.github.com") {
		t.Fatalf("the API was called even though the redirect resolved:\n%s", curlLog)
	}
}

// TestInstallScriptFallsBackToTheAPI is the mirror image: a network that passes
// the API but not the release pages still installs. The redirect is preferred,
// not required.
func TestInstallScriptFallsBackToTheAPI(t *testing.T) {
	root := repoRoot(t)
	env := newScriptEnv(t, "v1.2.3", "arm64")
	env.writeFakeUname("Darwin", "arm64")
	env.writeFakeCurl(curlFail, curlServe)
	env.writeFakeLimactl()

	out, err := env.run(root)
	if err != nil {
		t.Fatalf("install.sh failed with only the API reachable: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(env.installDir, "avm")); got != env.binaryContent {
		t.Fatalf("installed avm content = %q, want %q", got, env.binaryContent)
	}
}

// TestInstallScriptExplainsAFailedReleaseLookup pins the error. Every request in
// this script runs under curl -f, which prints nothing and returns 22 whatever
// the server said, so "failed to resolve latest avm release" left the reader with
// no way to tell a blocked host from a rate limit from a repository with no
// releases. When both ways are gone the script must report what each host
// answered and name the way past discovery altogether.
func TestInstallScriptExplainsAFailedReleaseLookup(t *testing.T) {
	root := repoRoot(t)
	env := newScriptEnv(t, "v1.2.3", "arm64")
	env.writeFakeUname("Darwin", "arm64")
	env.writeFakeCurl(curlFail, curlFail)
	env.writeFakeLimactl()

	out, err := env.run(root)
	if err == nil {
		t.Fatalf("install.sh succeeded with no reachable release source\n%s", out)
	}
	for _, want := range []string{
		"HTTP 404",     // what github.com answered
		"HTTP 403",     //   and the API, which is the whole diagnosis
		"AVM_VERSION=", // the way past release discovery
		"/releases",    // where to find a tag to pin
	} {
		if !strings.Contains(out, want) {
			t.Errorf("install.sh output does not mention %q:\n%s", want, out)
		}
	}
}

func TestInstallScriptFailsWhenLimaAndHomebrewAreMissing(t *testing.T) {
	root := repoRoot(t)
	env := newScriptEnv(t, "v1.2.3", "darwin", "arm64")
	env.writeFakeUname("Darwin", "arm64")
	env.writeFakeCurl(curlServe, curlServe)

	out, err := env.run(root)
	if err == nil {
		t.Fatalf("install.sh succeeded, want failure\n%s", out)
	}
	if !strings.Contains(out, "Lima is required") {
		t.Fatalf("output = %q, want Lima guidance", out)
	}
	if _, statErr := os.Stat(filepath.Join(env.installDir, "avm")); !os.IsNotExist(statErr) {
		t.Fatalf("avm installed despite missing Lima/Homebrew: %v", statErr)
	}
}

// curlMode says how the fake curl answers one of the two release-discovery
// endpoints — github.com's /releases/latest redirect and the API — so a test can
// reproduce a network that reaches one of them and not the other.
type curlMode string

const (
	curlServe curlMode = "serve" // answer normally
	curlFail  curlMode = "fail"  // refuse, the way curl -f exits on a 403
)

func TestInstallScriptInstallsLinuxArchive(t *testing.T) {
	root := repoRoot(t)
	env := newScriptEnv(t, "v1.2.3", "linux", "amd64")
	env.writeFakeUname("Linux", "x86_64")
	env.writeFakeCurl(curlServe, curlServe)
	env.writeFakeLimactl()
	writeExecutableFile(filepath.Join(env.binDir, "shasum"), "#!/bin/sh\nexit 99\n")

	out, err := env.run(root)
	if err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(env.installDir, "avm")); got != env.binaryContent {
		t.Fatalf("installed avm content = %q, want %q", got, env.binaryContent)
	}
}

type scriptEnv struct {
	tempDir       string
	binDir        string
	installDir    string
	version       string
	archivePath   string
	checksumsPath string
	latestPath    string
	brewLog       string
	curlLog       string
	binaryContent string
	extraEnv      []string
}

func newScriptEnv(t *testing.T, version, host, arch string) *scriptEnv {
	t.Helper()

	tempDir := t.TempDir()
	env := &scriptEnv{
		tempDir:       tempDir,
		version:       version,
		binDir:        filepath.Join(tempDir, "bin"),
		installDir:    filepath.Join(tempDir, "install"),
		brewLog:       filepath.Join(tempDir, "brew.log"),
		curlLog:       filepath.Join(tempDir, "curl.log"),
		latestPath:    filepath.Join(tempDir, "latest.json"),
		binaryContent: "#!/bin/sh\necho fake avm\n",
	}
	if err := os.MkdirAll(env.binDir, 0o755); err != nil {
		t.Fatalf("create fake bin dir: %v", err)
	}
	if err := os.WriteFile(env.latestPath, []byte(fmt.Sprintf(`{"tag_name":"%s"}`, version)), 0o644); err != nil {
		t.Fatalf("write latest json: %v", err)
	}
	env.archivePath, env.checksumsPath = writeReleaseFiles(t, tempDir, version, host, arch, env.binaryContent)
	return env
}

func (e *scriptEnv) run(root string) (string, error) {
	cmd := exec.Command("sh", filepath.Join(root, "install.sh"))
	cmd.Env = []string{
		"PATH=" + e.binDir + string(os.PathListSeparator) + strings.Join(systemPath(), string(os.PathListSeparator)),
		"HOME=" + filepath.Join(e.tempDir, "home"),
		"AVM_INSTALL_DIR=" + e.installDir,
		"FAKE_ARCHIVE=" + e.archivePath,
		"FAKE_CHECKSUMS=" + e.checksumsPath,
		"FAKE_LATEST_JSON=" + e.latestPath,
		"FAKE_LATEST_TAG=" + e.version,
		"FAKE_BREW_LOG=" + e.brewLog,
		"FAKE_CURL_LOG=" + e.curlLog,
	}
	cmd.Env = append(cmd.Env, e.extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func systemPath() []string {
	if runtime.GOOS == "windows" {
		return []string{os.Getenv("PATH")}
	}
	return []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"}
}

func (e *scriptEnv) writeFakeUname(osName, arch string) {
	writeExecutableFile(filepath.Join(e.binDir, "uname"), fmt.Sprintf(`#!/bin/sh
case "$1" in
  -s) printf '%%s\n' %q ;;
  -m) printf '%%s\n' %q ;;
  *) printf 'unsupported uname args: %%s\n' "$*" >&2; exit 2 ;;
esac
`, osName, arch))
}

// writeFakeCurl installs a curl that answers the four URLs install.sh fetches.
// It understands -o and -w with their arguments: release discovery asks for
// %{url_effective} (where the /releases/latest redirect landed) and, on the
// failure path, %{http_code}.
func (e *scriptEnv) writeFakeCurl(redirect, api curlMode) {
	e.extraEnv = append(e.extraEnv,
		"FAKE_REDIRECT_MODE="+string(redirect),
		"FAKE_API_MODE="+string(api))

	writeExecutableFile(filepath.Join(e.binDir, "curl"), `#!/bin/sh
set -eu
out=
url=
fmt=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      out=$2
      shift 2
      ;;
    -w)
      fmt=$2
      shift 2
      ;;
    -*)
      shift
      ;;
    *)
      url=$1
      shift
      ;;
  esac
done
printf '%s\n' "$url" >> "$FAKE_CURL_LOG"

# refuse <status> — a host that answers but not with the release. curl -f exits
# 22 and prints nothing; only an explicit -w %{http_code} reports the status.
refuse() {
  case "$fmt" in
    *http_code*) printf '%s\n' "$1"; exit 0 ;;
  esac
  exit 22
}

case "$url" in
  *api.github.com*/releases/latest)
    [ "$FAKE_API_MODE" = serve ] || refuse 403
    cat "$FAKE_LATEST_JSON"
    ;;
  https://github.com/*/releases/latest)
    [ "$FAKE_REDIRECT_MODE" = serve ] || refuse 404
    case "$fmt" in
      *url_effective*) printf '%s\n' "${url%/latest}/tag/$FAKE_LATEST_TAG" ;;
      *http_code*) printf '302\n' ;;
    esac
    ;;
  */checksums.txt)
    cp "$FAKE_CHECKSUMS" "$out"
    ;;
  */avm_*_*.tar.gz)
    cp "$FAKE_ARCHIVE" "$out"
    ;;
  *)
    printf 'unexpected curl url: %s\n' "$url" >&2
    exit 22
    ;;
esac
`)
}

func (e *scriptEnv) writeFakeBrew() {
	writeExecutableFile(filepath.Join(e.binDir, "brew"), `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$FAKE_BREW_LOG"
`)
}

func (e *scriptEnv) writeFakeLimactl() {
	writeExecutableFile(filepath.Join(e.binDir, "limactl"), `#!/bin/sh
exit 0
`)
}

func writeReleaseFiles(t *testing.T, dir, version, host, arch, binaryContent string) (string, string) {
	t.Helper()

	archiveName := fmt.Sprintf("avm_%s_%s_%s.tar.gz", strings.TrimPrefix(version, "v"), host, arch)
	archivePath := filepath.Join(dir, archiveName)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	data := []byte(binaryContent)
	if err := tw.WriteHeader(&tar.Header{Name: "avm", Mode: 0o755, Size: int64(len(data))}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	sum := sha256.Sum256(buf.Bytes())
	checksumsPath := filepath.Join(dir, "checksums.txt")
	checksums := fmt.Sprintf("%x  %s\n", sum, archiveName)
	if err := os.WriteFile(checksumsPath, []byte(checksums), 0o644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}

	return archivePath, checksumsPath
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func assertExecutable(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("%s is not executable: mode %v", path, info.Mode())
	}
}

func writeExecutableFile(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		panic(err)
	}
}
