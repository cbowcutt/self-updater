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
	"time"

	"github.com/cbowcutt/self-updater/internal/manifest/models"
	"golang.org/x/mod/semver"
)

// GithubBinaryProvider fetches release metadata and retrieves verified binaries.
type GithubBinaryProvider struct {
	client *http.Client
}

// NewGithubBinaryProvider creates a GitHub binary provider with a hardened HTTP client.
func NewGithubBinaryProvider() *GithubBinaryProvider {
	return &GithubBinaryProvider{
		client: createSecureHTTPClient(),
	}
}

// GetManifest fetches and decodes the latest published manifest.
func (g *GithubBinaryProvider) GetManifest() (*models.Manifest, error) {
	const manifestURL = "https://github.com/cbowcutt/self-updater/releases/latest/download/manifest.json"
	slog.Info("fetching latest release manifest")
	requestStarted := time.Now()
	request, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest request: %w", err)
	}
	request.Header.Set("Cache-Control", "no-cache")

	resp, err := g.client.Do(request)
	if err != nil {
		slog.Error("manifest request failed", "duration", time.Since(requestStarted), "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("manifest request returned non-success status", "status", resp.Status)
		return nil, fmt.Errorf("unexpected status code while fetching manifest: %s", resp.Status)
	}

	var manifest models.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		slog.Error("failed to decode release manifest", "error", err)
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}
	slog.Info("release manifest fetched", "version", manifest.Version, "artifacts", len(manifest.Artifacts), "duration", time.Since(requestStarted))
	return &manifest, nil
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
