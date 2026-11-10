package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/cbowcutt/self-updater/internal/manifest/models"
	"golang.org/x/mod/semver"
)

// Manifest is the manifest model produced by the GitHub release metadata.
type Manifest = models.Manifest

// GithubManifestProvider fetches release manifests and binaries from GitHub releases.
type GithubManifestProvider struct {
	client *http.Client
}

// NewManifestProvider creates a GitHub-backed manifest provider with a hardened HTTP client.
func NewManifestProvider() *GithubManifestProvider {
	return &GithubManifestProvider{client: createSecureHttpClient()}
}

// GetManifest fetches and decodes the latest published manifest.
func (g *GithubManifestProvider) GetManifest() (*Manifest, error) {
	resp, err := g.client.Get("https://github.com/cbowcutt/self-updater/releases/latest/download/manifest.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code while fetching manifest: %s", resp.Status)
	}
	decoder := json.NewDecoder(resp.Body)

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}
	return &manifest, nil
}

// GetExecutable downloads the artifact for the requested runtime and architecture.
func (g *GithubManifestProvider) GetExecutable(manifest *Manifest, runtime string, architecture string) ([]byte, error) {
	url := manifest.Artifacts[runtime+"-"+architecture].URL
	resp, err := http.Get(url)
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
	return body, nil
}

func isNewer(remote, current string) (bool, error) {
	r, c := "v"+remote, "v"+current
	if !semver.IsValid(r) || !semver.IsValid(c) {
		return false, fmt.Errorf("invalid version: remote=%q current=%q", remote, current)
	}
	return semver.Compare(r, c) > 0, nil
}

func createSecureHttpClient() *http.Client {
	customTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,  // Limit TCP connection setups
			KeepAlive: 30 * time.Second, // Keep-alive probe interval
		}).DialContext,
		MaxIdleConns:          100,              // Total global idle connections
		MaxIdleConnsPerHost:   25,               // Increase for microservices
		IdleConnTimeout:       90 * time.Second, // Drop inactive links
		TLSHandshakeTimeout:   5 * time.Second,  // Restrict TLS negotiations
		ExpectContinueTimeout: 1 * time.Second,
	}

	secureHTTPClient := &http.Client{
		Transport: customTransport,
		Timeout:   15 * time.Second, // Global timeout bucket for entire transaction
	}
	return secureHTTPClient
}
