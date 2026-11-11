package binaryProvider

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/cbowcutt/self-updater/internal/manifest/models"
)

const latestReleaseAPIURL = "https://api.github.com/repos/cbowcutt/self-updater/releases?per_page=100&page=1"
const latestManifestURL = "https://github.com/cbowcutt/self-updater/releases/download/v1.10.0/manifest.json"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
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
		{name: "older", remote: "1.0.0", current: "2.0.0"},
		{name: "same", remote: "1.0.0", current: "1.0.0"},
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
			if err != nil || got != tc.want {
				t.Fatalf("isNewer(%q, %q) = %v, %v; want %v", tc.remote, tc.current, got, err, tc.want)
			}
		})
	}
}

func TestGetBinary(t *testing.T) {
	const body = "hello binary"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	provider := &GithubBinaryProvider{client: server.Client()}
	manifest := &models.Manifest{Artifacts: map[string]models.Artifact{
		"linux-amd64": {URL: server.URL, SHA256: checksum([]byte(body))},
	}}
	got, err := provider.GetBinary(manifest, "linux", "amd64")
	if err != nil {
		t.Fatalf("GetBinary returned error: %v", err)
	}
	if string(got) != body {
		t.Fatalf("binary = %q, want %q", got, body)
	}
}

func TestGetManifest(t *testing.T) {
	provider := &GithubBinaryProvider{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == latestReleaseAPIURL {
			if req.Header.Get("Accept") != "application/vnd.github+json" || req.Header.Get("Cache-Control") != "no-cache" {
				t.Fatalf("unexpected release API headers: %v", req.Header)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`[
					{"tag_name":"v1.2.3","assets":[{"name":"manifest.json","browser_download_url":"https://example.com/v1.2.3/manifest.json"}]},
					{"tag_name":"release-notes","assets":[]},
					{"tag_name":"v99.0.0","draft":true,"assets":[]},
					{"tag_name":"v1.10.0","assets":[{"name":"manifest.json","browser_download_url":"` + latestManifestURL + `"}]}
				]`)),
			}, nil
		}
		if req.URL.String() != latestManifestURL {
			t.Fatalf("unexpected URL: %s", req.URL.String())
		}
		if req.Header.Get("Cache-Control") != "no-cache" {
			t.Fatalf("manifest request missing no-cache header: %v", req.Header)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"version":"1.10.0","artifacts":{"linux-amd64":{"url":"https://example.com/bin","sha256":"abc"}}}`)),
		}, nil
	})}}

	manifest, err := provider.GetManifest()
	if err != nil {
		t.Fatalf("GetManifest returned error: %v", err)
	}
	if manifest.Version != "1.10.0" || manifest.Artifacts["linux-amd64"].URL != "https://example.com/bin" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
}

func TestGetManifestRejectsBadStatus(t *testing.T) {
	provider := &GithubBinaryProvider{client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("boom")),
		}, nil
	})}}
	if _, err := provider.GetManifest(); err == nil || !strings.Contains(err.Error(), "unexpected status") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestGetManifestSelectsHighestSemverAcrossPages(t *testing.T) {
	const manifestURL = "https://github.com/cbowcutt/self-updater/releases/download/v1.0.0/manifest.json"
	var firstPage strings.Builder
	firstPage.WriteByte('[')
	for i := 1; i <= 100; i++ {
		if i > 1 {
			firstPage.WriteByte(',')
		}
		fmt.Fprintf(&firstPage, `{"tag_name":"v0.0.%d","assets":[]}`, i)
	}
	firstPage.WriteByte(']')

	provider := &GithubBinaryProvider{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Query().Get("page") {
		case "1":
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(firstPage.String()))}, nil
		case "2":
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[{"tag_name":"v1.0.0","assets":[{"name":"manifest.json","browser_download_url":"` + manifestURL + `"}]}]`))}, nil
		default:
			if req.URL.String() == manifestURL {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"version":"1.0.0","artifacts":{"linux-amd64":{"url":"https://example.com/bin","sha256":"abc"}}}`))}, nil
			}
			t.Fatalf("unexpected request: %s", req.URL)
			return nil, nil
		}
	})}}

	manifest, err := provider.GetManifest()
	if err != nil {
		t.Fatalf("GetManifest returned error: %v", err)
	}
	if manifest.Version != "1.0.0" {
		t.Fatalf("selected manifest version = %q, want 1.0.0", manifest.Version)
	}
}

func TestGetManifestRejectsBadManifestJSON(t *testing.T) {
	provider := &GithubBinaryProvider{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == latestReleaseAPIURL {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`[{"tag_name":"v1.2.3","assets":[{"name":"manifest.json","browser_download_url":"` + latestManifestURL + `"}]}]`)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("not-json")),
		}, nil
	})}}
	if _, err := provider.GetManifest(); err == nil || !strings.Contains(err.Error(), "failed to decode manifest") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestGetManifestRejectsVersionMismatch(t *testing.T) {
	provider := &GithubBinaryProvider{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == latestReleaseAPIURL {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[{"tag_name":"v1.2.3","assets":[{"name":"manifest.json","browser_download_url":"` + latestManifestURL + `"}]}]`))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"version":"1.2.2","artifacts":{"linux-amd64":{"url":"https://example.com/bin","sha256":"abc"}}}`))}, nil
	})}}
	if _, err := provider.GetManifest(); err == nil || !strings.Contains(err.Error(), "does not match latest release tag") {
		t.Fatalf("expected release version mismatch error, got %v", err)
	}
}

func TestGetBinaryRejectsChecksumMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer server.Close()

	provider := &GithubBinaryProvider{client: server.Client()}
	manifest := &models.Manifest{Artifacts: map[string]models.Artifact{
		"linux-amd64": {URL: server.URL, SHA256: checksum([]byte("expected"))},
	}}
	if _, err := provider.GetBinary(manifest, "linux", "amd64"); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestGetBinaryRejectsBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad", http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := &GithubBinaryProvider{client: server.Client()}
	manifest := &models.Manifest{Artifacts: map[string]models.Artifact{
		"linux-amd64": {URL: server.URL, SHA256: checksum(nil)},
	}}
	if _, err := provider.GetBinary(manifest, "linux", "amd64"); err == nil || !strings.Contains(err.Error(), "unexpected status code") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestGetBinaryRejectsMissingArtifact(t *testing.T) {
	provider := &GithubBinaryProvider{client: http.DefaultClient}
	if _, err := provider.GetBinary(&models.Manifest{Artifacts: map[string]models.Artifact{}}, "linux", "amd64"); err == nil {
		t.Fatal("want error when artifact is missing")
	}
}

func TestCheckAndDownloadLatestVersion(t *testing.T) {
	const body = "verified executable"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	platform := runtime.GOOS + "-" + runtime.GOARCH
	const releaseManifestURL = "https://github.com/cbowcutt/self-updater/releases/download/v2.0.0/manifest.json"
	manifestData := `{"version":"2.0.0","artifacts":{"` + platform + `":{"url":"` + server.URL + `","sha256":"` + checksum([]byte(body)) + `"}}}`
	provider := &GithubBinaryProvider{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == latestReleaseAPIURL {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[{"tag_name":"v2.0.0","assets":[{"name":"manifest.json","browser_download_url":"` + releaseManifestURL + `"}]}]`))}, nil
		}
		if req.URL.String() == releaseManifestURL {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(manifestData))}, nil
		}
		return server.Client().Transport.RoundTrip(req)
	})}}
	shouldUpdate, binary, err := provider.CheckAndDownloadLatestVersion("1.0.0")
	if err != nil {
		t.Fatalf("CheckAndDownloadLatestVersion returned error: %v", err)
	}
	if !shouldUpdate {
		t.Fatal("shouldUpdate = false, want true")
	}
	if binary == nil || string(*binary) != body {
		t.Fatalf("binary = %v, want %q", binary, body)
	}
}

func TestCheckAndDownloadLatestVersionReturnsManifestError(t *testing.T) {
	wantErr := errors.New("manifest fetch failed")
	provider := &GithubBinaryProvider{client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, wantErr
	})}}
	if _, _, err := provider.CheckAndDownloadLatestVersion("1.0.0"); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}
