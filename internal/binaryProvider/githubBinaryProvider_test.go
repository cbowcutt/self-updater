package binaryProvider

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/cbowcutt/self-updater/internal/manifest/models"
)

const latestManifestURL = "https://github.com/cbowcutt/self-updater/releases/latest/download/manifest.json"

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
		if req.URL.String() != latestManifestURL {
			t.Fatalf("unexpected URL: %s", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"version":"1.2.3","artifacts":{"linux-amd64":{"url":"https://example.com/bin","sha256":"abc"}}}`)),
		}, nil
	})}}

	manifest, err := provider.GetManifest()
	if err != nil {
		t.Fatalf("GetManifest returned error: %v", err)
	}
	if manifest.Version != "1.2.3" || manifest.Artifacts["linux-amd64"].URL != "https://example.com/bin" {
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
	if _, err := provider.GetManifest(); err == nil || !strings.Contains(err.Error(), "unexpected status code") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestGetManifestRejectsBadJSON(t *testing.T) {
	provider := &GithubBinaryProvider{client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
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
	manifestData := `{"version":"2.0.0","artifacts":{"` + platform + `":{"url":"` + server.URL + `","sha256":"` + checksum([]byte(body)) + `"}}}`
	provider := &GithubBinaryProvider{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == latestManifestURL {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(manifestData))}, nil
		}
		return server.Client().Transport.RoundTrip(req)
	})}}
	binary, err := provider.CheckAndDownloadLatestVersion("1.0.0")
	if err != nil {
		t.Fatalf("CheckAndDownloadLatestVersion returned error: %v", err)
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
	if _, err := provider.CheckAndDownloadLatestVersion("1.0.0"); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}
