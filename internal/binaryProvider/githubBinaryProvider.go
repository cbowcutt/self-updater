package binaryProvider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/cbowcutt/self-updater/internal/manifest/models"
	"golang.org/x/mod/semver"
)

// GithubBinaryProvider fetches release metadata and retrieves verified binaries.
type GithubBinaryProvider struct {
	client *http.Client
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Draft   bool   `json:"draft"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// NewGithubBinaryProvider creates a GitHub binary provider with a hardened HTTP client.
func NewGithubBinaryProvider() *GithubBinaryProvider {
	return &GithubBinaryProvider{
		client: createSecureHTTPClient(),
	}
}

// GetManifest selects the release with the highest semantic-version tag from
// GitHub's release list and downloads that release's version-specific manifest.
func (g *GithubBinaryProvider) GetManifest() (*models.Manifest, error) {
	requestStarted := time.Now()
	release, err := g.latestReleaseBySemver()
	if err != nil {
		return nil, err
	}

	var manifestURL string
	for _, asset := range release.Assets {
		if asset.Name == "manifest.json" {
			manifestURL = asset.BrowserDownloadURL
			break
		}
	}
	if manifestURL == "" {
		return nil, fmt.Errorf("latest GitHub release %q has no manifest.json asset", release.TagName)
	}

	slog.Info("fetching manifest for latest GitHub release", "tag", release.TagName, "url", manifestURL)
	manifestRequest, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest request: %w", err)
	}
	manifestRequest.Header.Set("Cache-Control", "no-cache")
	manifestResp, err := g.client.Do(manifestRequest)
	if err != nil {
		slog.Error("manifest request failed", "tag", release.TagName, "duration", time.Since(requestStarted), "error", err)
		return nil, fmt.Errorf("fetch manifest for release %q: %w", release.TagName, err)
	}
	defer manifestResp.Body.Close()
	if manifestResp.StatusCode != http.StatusOK {
		slog.Warn("manifest request returned non-success status", "tag", release.TagName, "status", manifestResp.Status)
		return nil, fmt.Errorf("unexpected status while fetching manifest for release %q: %s", release.TagName, manifestResp.Status)
	}

	var manifest models.Manifest
	if err := json.NewDecoder(manifestResp.Body).Decode(&manifest); err != nil {
		slog.Error("failed to decode manifest", "tag", release.TagName, "error", err)
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}
	if release.TagName != "" && strings.TrimPrefix(release.TagName, "v") != manifest.Version {
		return nil, fmt.Errorf("manifest version %q does not match latest release tag %q", manifest.Version, release.TagName)
	}
	slog.Info("release manifest fetched", "version", manifest.Version, "tag", release.TagName, "artifacts", len(manifest.Artifacts), "duration", time.Since(requestStarted))
	return &manifest, nil
}

func (g *GithubBinaryProvider) latestReleaseBySemver() (*githubRelease, error) {
	const releasesURL = "https://api.github.com/repos/cbowcutt/self-updater/releases"
	const pageSize = 100

	slog.Info("fetching GitHub releases to select highest semantic version")
	var latest *githubRelease
	latestVersion := ""
	for page := 1; ; page++ {
		url := fmt.Sprintf("%s?per_page=%d&page=%d", releasesURL, pageSize, page)
		request, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitHub releases request: %w", err)
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		request.Header.Set("Cache-Control", "no-cache")

		resp, err := g.client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("fetch GitHub releases page %d: %w", page, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("unexpected status fetching GitHub releases page %d: %s", page, resp.Status)
		}

		var releases []githubRelease
		decodeErr := json.NewDecoder(resp.Body).Decode(&releases)
		closeErr := resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("decode GitHub releases page %d: %w", page, decodeErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close GitHub releases page %d: %w", page, closeErr)
		}

		for i := range releases {
			if releases[i].Draft {
				continue
			}
			version := semverTag(releases[i].TagName)
			if version == "" {
				slog.Warn("skipping release with non-semver tag", "tag", releases[i].TagName)
				continue
			}
			if latest == nil || semver.Compare(version, latestVersion) > 0 {
				latest = &releases[i]
				latestVersion = version
			}
		}

		if len(releases) < pageSize {
			break
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("no published releases with valid semantic-version tags found")
	}
	slog.Info("selected highest semantic-version release", "tag", latest.TagName)
	return latest, nil
}

func semverTag(tag string) string {
	version := "v" + strings.TrimPrefix(tag, "v")
	if !semver.IsValid(version) {
		return ""
	}
	return version
}

// IsNewerVersionAvailable reports whether a newer remote release is available.
func (g *GithubBinaryProvider) IsNewerVersionAvailable() (bool, error) {
	return true, nil
}

// CheckAndDownloadLatestVersion checks for a newer release and retrieves its verified binary.
func (g *GithubBinaryProvider) CheckAndDownloadLatestVersion(currentVersion string) (bool, *[]byte, error) {
	slog.Info("checking for an update", "current_version", currentVersion, "platform", runtime.GOOS+"-"+runtime.GOARCH)
	manifest, err := g.GetManifest()
	if err != nil {
		return false, nil, err
	}
	newer, err := isNewer(manifest.Version, currentVersion)
	if err != nil {
		slog.Error("cannot compare release versions", "current_version", currentVersion, "release_version", manifest.Version, "error", err)
		return false, nil, err
	}
	if !newer {
		slog.Info("no update available", "current_version", currentVersion, "release_version", manifest.Version)
		return false, nil, nil
	}
	slog.Info("newer release available", "current_version", currentVersion, "release_version", manifest.Version)

	binary, err := g.GetBinary(manifest, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		slog.Error("failed to download available release", "version", manifest.Version, "error", err)
		return false, nil, err
	}

	slog.Info("update binary downloaded and verified", "version", manifest.Version, "bytes", len(binary))
	return true, &binary, nil
}

// GetBinary downloads and verifies the artifact for the requested runtime and architecture.
func (g *GithubBinaryProvider) GetBinary(manifest *models.Manifest, goos string, goarch string) ([]byte, error) {
	platform := goos + "-" + goarch
	slog.Info("downloading release binary", "platform", platform)
	artifact, ok := manifest.Artifacts[platform]
	if !ok {
		slog.Warn("release manifest does not contain this platform", "platform", platform)
		return nil, fmt.Errorf("manifest has no artifact for %s", platform)
	}

	requestStarted := time.Now()
	resp, err := g.client.Get(artifact.URL)
	if err != nil {
		slog.Error("binary request failed", "platform", platform, "duration", time.Since(requestStarted), "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("binary request returned non-success status", "platform", platform, "status", resp.Status)
		return nil, fmt.Errorf("unexpected status code while fetching binary: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("failed to read downloaded binary", "platform", platform, "error", err)
		return nil, fmt.Errorf("failed to read binary: %w", err)
	}
	if err := verifyChecksum(body, artifact.SHA256); err != nil {
		slog.Error("downloaded binary checksum verification failed", "platform", platform, "error", err)
		return nil, fmt.Errorf("checksum mismatch for %s: %w", platform, err)
	}
	slog.Info("downloaded binary checksum verified", "platform", platform, "bytes", len(body), "duration", time.Since(requestStarted))
	return body, nil
}

func verifyChecksum(data []byte, expectedSHA256 string) error {
	actualSHA256 := sha256.Sum256(data)
	if got := hex.EncodeToString(actualSHA256[:]); got != expectedSHA256 {
		return fmt.Errorf("got %s, want %s", got, expectedSHA256)
	}
	return nil
}

func isNewer(remote, current string) (bool, error) {
	r := "v" + remote
	c := "v" + current
	if !semver.IsValid(r) || (!semver.IsValid(c) && c != "vdev") {
		return false, fmt.Errorf("invalid version: remote=%q current=%q", remote, current)
	}
	if c == "vdev" {
		return true, nil
	}
	return semver.Compare(r, c) > 0, nil
}

func createSecureHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   25,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second}
}
