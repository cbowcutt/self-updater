package selfUpdater

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/cbowcutt/self-updater/internal/manifest/models"
)

type stubManifestProvider struct {
	manifest     *Manifest
	manifestErr  error
	execErr      error
	execCalled   bool
	lastRuntime  string
	lastArch     string
	lastManifest *Manifest
}

func (s *stubManifestProvider) GetManifest() (*Manifest, error) {
	if s.manifestErr != nil {
		return nil, s.manifestErr
	}
	return s.manifest, nil
}

func (s *stubManifestProvider) GetExecutable(manifest *Manifest, runtime string, architecture string) ([]byte, error) {
	s.execCalled = true
	s.lastRuntime = runtime
	s.lastArch = architecture
	s.lastManifest = manifest
	return []byte("downloaded"), s.execErr
}

func TestIsNewer(t *testing.T) {
	for _, tc := range []struct {
		name    string
		remote  string
		current string
		want    bool
		wantErr bool
	}{
		{name: "newer", remote: "2.0.0", current: "1.9.9", want: true},
		{name: "older", remote: "1.0.0", current: "2.0.0", want: false},
		{name: "same", remote: "1.0.0", current: "1.0.0", want: false},
		{name: "invalid remote", remote: "bad", current: "1.0.0", wantErr: true},
		{name: "invalid current", remote: "1.0.0", current: "bad", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := isNewer(tc.remote, tc.current)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("isNewer(%q, %q) = %v, want %v", tc.remote, tc.current, got, tc.want)
			}
		})
	}
}

func TestGithubSelfUpdaterIsNewerVersionAvailable(t *testing.T) {
	updater := NewGithubSelfUpdater(&stubManifestProvider{})
	got, err := updater.IsNewerVersionAvailable()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatal("IsNewerVersionAvailable() = false, want true")
	}
}

func TestGithubSelfUpdaterCheckAndDownloadLatestVersionDownloadsWhenNewer(t *testing.T) {
	provider := &stubManifestProvider{
		manifest: &Manifest{Version: "2.0.0", PublishedAt: time.Unix(0, 0), Artifacts: map[string]models.Artifact{"linux-amd64": {URL: "https://example.com/v2", SHA256: "hash"}}},
	}
	updater := NewGithubSelfUpdater(provider)

	if err := updater.CheckAndDownloadLatestVersion("1.0.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !provider.execCalled {
		t.Fatal("expected executable download to run")
	}
	if provider.lastRuntime != runtime.GOOS || provider.lastArch != runtime.GOARCH {
		t.Fatalf("download runtime/arch = %s/%s, want %s/%s", provider.lastRuntime, provider.lastArch, runtime.GOOS, runtime.GOARCH)
	}
}

func TestGithubSelfUpdaterCheckAndDownloadLatestVersionSkipsWhenOlder(t *testing.T) {
	provider := &stubManifestProvider{
		manifest: &Manifest{Version: "1.0.0", PublishedAt: time.Unix(0, 0), Artifacts: map[string]models.Artifact{"linux-amd64": {URL: "https://example.com/v1", SHA256: "hash"}}},
	}
	updater := NewGithubSelfUpdater(provider)

	if err := updater.CheckAndDownloadLatestVersion("2.0.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.execCalled {
		t.Fatal("download should not run when current version is newer or equal")
	}
}

func TestGithubSelfUpdaterCheckAndDownloadLatestVersionReturnsManifestError(t *testing.T) {
	wantErr := errors.New("fetch failed")
	updater := NewGithubSelfUpdater(&stubManifestProvider{manifestErr: wantErr})

	if err := updater.CheckAndDownloadLatestVersion("1.0.0"); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestGithubSelfUpdaterCheckAndDownloadLatestVersionReturnsDownloadError(t *testing.T) {
	wantErr := errors.New("download failed")
	provider := &stubManifestProvider{
		manifest: &Manifest{Version: "2.0.0", PublishedAt: time.Unix(0, 0), Artifacts: map[string]models.Artifact{"linux-amd64": {URL: "https://example.com/v2", SHA256: "hash"}}},
		execErr:  wantErr,
	}
	updater := NewGithubSelfUpdater(provider)

	if err := updater.CheckAndDownloadLatestVersion("1.0.0"); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestGithubSelfUpdaterCheckAndDownloadLatestVersionRejectsInvalidRemoteVersion(t *testing.T) {
	provider := &stubManifestProvider{manifest: &Manifest{Version: "not-semver"}}
	updater := NewGithubSelfUpdater(provider)

	if err := updater.CheckAndDownloadLatestVersion("1.0.0"); err == nil {
		t.Fatal("want error for invalid remote semver")
	}
	if provider.execCalled {
		t.Fatal("should not attempt download when remote version is invalid")
	}
}
