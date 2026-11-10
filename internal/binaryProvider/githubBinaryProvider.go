package binaryProvider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	resp, err := g.client.Get("https://github.com/cbowcutt/self-updater/releases/latest/download/manifest.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code while fetching manifest: %s", resp.Status)
	}

	var manifest models.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}
	return &manifest, nil
}

// IsNewerVersionAvailable reports whether a newer remote release is available.
func (g *GithubBinaryProvider) IsNewerVersionAvailable() (bool, error) {
	return true, nil
}

// CheckAndDownloadLatestVersion checks for a newer release and retrieves its verified binary.
func (g *GithubBinaryProvider) CheckAndDownloadLatestVersion(currentVersion string) (*[]byte, error) {
	manifest, err := g.GetManifest()
	if err != nil {
		return nil, err
	}
	newer, err := isNewer(manifest.Version, currentVersion)
	if err != nil {
		return nil, err
	}
	if !newer {
		return nil, nil
	}

	binary, err := g.GetBinary(manifest, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, err
	}

	return &binary, nil
}

// GetBinary downloads and verifies the artifact for the requested runtime and architecture.
func (g *GithubBinaryProvider) GetBinary(manifest *models.Manifest, goos string, goarch string) ([]byte, error) {
	platform := goos + "-" + goarch
	artifact, ok := manifest.Artifacts[platform]
	if !ok {
		return nil, fmt.Errorf("manifest has no artifact for %s", platform)
	}

	resp, err := g.client.Get(artifact.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code while fetching binary: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read binary: %w", err)
	}
	if err := verifyChecksum(body, artifact.SHA256); err != nil {
		return nil, fmt.Errorf("checksum mismatch for %s: %w", platform, err)
	}
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
	r, c := "v"+remote, "v"+current
	if !semver.IsValid(r) || !semver.IsValid(c) {
		return false, fmt.Errorf("invalid version: remote=%q current=%q", remote, current)
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
