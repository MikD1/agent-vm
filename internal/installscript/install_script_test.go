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
	env := newScriptEnv(t, "v1.2.3", "arm64")
	env.writeFakeUname("Darwin", "arm64")
	env.writeFakeCurl(false)
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
	env := newScriptEnv(t, "v9.8.7", "amd64")
	env.writeFakeUname("Darwin", "x86_64")
	env.writeFakeCurl(true)
	env.writeFakeLimactl()
	env.extraEnv = append(env.extraEnv, "AVM_VERSION=v9.8.7")

	out, err := env.run(root)
	if err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, out)
	}

	curlLog := readFile(t, env.curlLog)
	if strings.Contains(curlLog, "api.github.com") {
		t.Fatalf("latest release API was called for pinned install:\n%s", curlLog)
	}

	installed := filepath.Join(env.installDir, "avm")
	if got := readFile(t, installed); got != env.binaryContent {
		t.Fatalf("installed avm content = %q, want %q", got, env.binaryContent)
	}
}

func TestInstallScriptFailsWhenLimaAndHomebrewAreMissing(t *testing.T) {
	root := repoRoot(t)
	env := newScriptEnv(t, "v1.2.3", "arm64")
	env.writeFakeUname("Darwin", "arm64")
	env.writeFakeCurl(false)

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

type scriptEnv struct {
	tempDir       string
	binDir        string
	installDir    string
	archivePath   string
	checksumsPath string
	latestPath    string
	brewLog       string
	curlLog       string
	binaryContent string
	extraEnv      []string
}

func newScriptEnv(t *testing.T, version, arch string) *scriptEnv {
	t.Helper()

	tempDir := t.TempDir()
	env := &scriptEnv{
		tempDir:       tempDir,
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
	env.archivePath, env.checksumsPath = writeReleaseFiles(t, tempDir, version, arch, env.binaryContent)
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

func (e *scriptEnv) writeFakeCurl(failLatest bool) {
	latestCase := `cat "$FAKE_LATEST_JSON"`
	if failLatest {
		latestCase = `printf 'unexpected latest lookup\n' >&2; exit 9`
	}

	writeExecutableFile(filepath.Join(e.binDir, "curl"), fmt.Sprintf(`#!/bin/sh
set -eu
out=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      out=$2
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
printf '%%s\n' "$url" >> "$FAKE_CURL_LOG"
case "$url" in
  *api.github.com*/releases/latest)
    %s
    ;;
  */checksums.txt)
    cp "$FAKE_CHECKSUMS" "$out"
    ;;
  */avm_*_darwin_*.tar.gz)
    cp "$FAKE_ARCHIVE" "$out"
    ;;
  *)
    printf 'unexpected curl url: %%s\n' "$url" >&2
    exit 22
    ;;
esac
`, latestCase))
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

func writeReleaseFiles(t *testing.T, dir, version, arch, binaryContent string) (string, string) {
	t.Helper()

	archiveName := fmt.Sprintf("avm_%s_darwin_%s.tar.gz", strings.TrimPrefix(version, "v"), arch)
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
