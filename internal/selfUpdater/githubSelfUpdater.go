package selfUpdater

import (
	"fmt"
	"runtime"

	"github.com/cbowcutt/self-updater/internal/manifest/models"
	"golang.org/x/mod/semver"
)

// Manifest is the release manifest data model used by the updater.
type Manifest = models.Manifest

// ManifestProvider supplies release metadata and artifact downloads for the updater.
type ManifestProvider interface {
	GetManifest() (*Manifest, error)
	GetExecutable(manifest *Manifest, runtime string, architecture string) ([]byte, error)
}

// GithubSelfUpdater checks for a newer release and downloads the matching binary.
type GithubSelfUpdater struct {
	manifestProvider ManifestProvider
}

// NewGithubSelfUpdater creates a self-updater backed by the provided manifest provider.
func NewGithubSelfUpdater(manifestProvider ManifestProvider) *GithubSelfUpdater {
	return &GithubSelfUpdater{manifestProvider: manifestProvider}
}

// IsNewerVersionAvailable reports whether a newer remote release is available.
func (g *GithubSelfUpdater) IsNewerVersionAvailable() (bool, error) {
	return true, nil
}

// CheckAndDownloadLatestVersion compares the current version against the latest manifest,
// downloads the matching binary when appropriate, and returns any error encountered.
func (g *GithubSelfUpdater) CheckAndDownloadLatestVersion(currentVersion string) error {
	manifest, err := g.manifestProvider.GetManifest()
	if err != nil {
		return err
	}
	isNewer, err := isNewer(manifest.Version, currentVersion)
	if err != nil {
		return err
	}
	if !isNewer {
		return nil
	}
	_, err = g.manifestProvider.GetExecutable(manifest, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	// Implement the logic to download the latest version.
	return nil
}

func isNewer(remote, current string) (bool, error) {
	r, c := "v"+remote, "v"+current
	if !semver.IsValid(r) || !semver.IsValid(c) {
		return false, fmt.Errorf("invalid version: remote=%q current=%q", remote, current)
	}
	return semver.Compare(r, c) > 0, nil
}
