package manifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuild(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app-linux-amd64", "linux-bytes")
	writeFile(t, dir, "app-windows-amd64.exe", "windows-bytes")
	// Files that must be ignored.
	writeFile(t, dir, "manifest.json", "{}")
	writeFile(t, dir, "manifest.json.sig", "abc")
	writeFile(t, dir, "README.md", "hi")

	now := time.Date(2026, 11, 10, 14, 0, 0, 0, time.UTC)
	m, err := Build(dir, "1.4.2", "https://example.com/releases/v1.4.2", now)
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Artifacts) != 2 {
		t.Fatalf("got %d artifacts, want 2: %v", len(m.Artifacts), m.Artifacts)
	}

	sum := sha256.Sum256([]byte("linux-bytes"))
	got := m.Artifacts["linux-amd64"]
	if got.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("linux sha256 = %s", got.SHA256)
	}
	if want := "https://example.com/releases/v1.4.2/app-linux-amd64"; got.URL != want {
		t.Errorf("linux url = %s, want %s", got.URL, want)
	}

	win, ok := m.Artifacts["windows-amd64"]
	if !ok {
		t.Fatal("missing windows-amd64 (.exe suffix should not leak into the key)")
	}
	if want := "https://example.com/releases/v1.4.2/app-windows-amd64.exe"; win.URL != want {
		t.Errorf("windows url = %s, want %s", win.URL, want)
	}
}

func TestBuildRejectsBadVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app-linux-amd64", "x")

	for _, v := range []string{"", "v1.0.0", "1.0", "1.0.0.0", "01.0.0", "latest"} {
		if _, err := Build(dir, v, "https://x", time.Now()); err == nil {
			t.Errorf("version %q accepted, want error", v)
		}
	}
	for _, v := range []string{"1.0.0", "0.9.12", "2.0.0-rc.1", "1.2.3+build.5"} {
		if _, err := Build(dir, v, "https://x", time.Now()); err != nil {
			t.Errorf("version %q rejected: %v", v, err)
		}
	}
}

func TestBuildNoArtifacts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "manifest.json", "{}")
	if _, err := Build(dir, "1.0.0", "https://x", time.Now()); err == nil {
		t.Fatal("want error when no binaries are present")
	}
}

func TestBuildDuplicatePlatform(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app-windows-amd64", "a")
	writeFile(t, dir, "app-windows-amd64.exe", "b")
	if _, err := Build(dir, "1.0.0", "https://x", time.Now()); err == nil {
		t.Fatal("want error for two binaries mapping to the same platform")
	}
}

func TestMarshalDeterministicAndParseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app-linux-amd64", "a")
	writeFile(t, dir, "app-darwin-arm64", "b")
	writeFile(t, dir, "app-windows-amd64.exe", "c")

	m, err := Build(dir, "1.0.0", "https://x", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := m.Marshal()
	b, _ := m.Marshal()
	if !bytes.Equal(a, b) {
		t.Fatal("Marshal is not deterministic; signatures would be unstable")
	}

	parsed, err := Parse(a)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Version != "1.0.0" || len(parsed.Artifacts) != 3 {
		t.Fatalf("round trip lost data: %+v", parsed)
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"not json":     `nope`,
		"bad version":  `{"version":"v1","artifacts":{"linux-amd64":{"url":"u","sha256":"h"}}}`,
		"no artifacts": `{"version":"1.0.0","artifacts":{}}`,
	}
	for name, in := range cases {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
