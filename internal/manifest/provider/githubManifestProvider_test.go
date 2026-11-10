package provider

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cbowcutt/self-updater/internal/manifest/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
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
		{name: "invalid remote", remote: "oops", current: "1.0.0", wantErr: true},
		{name: "invalid current", remote: "1.0.0", current: "oops", wantErr: true},
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

func TestCreateSecureHttpClient(t *testing.T) {
	client := createSecureHttpClient()
	if client == nil {
		t.Fatal("client is nil")
	}
	if client.Timeout != 15*time.Second {
		t.Fatalf("timeout = %s, want 15s", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.TLSHandshakeTimeout != 5*time.Second {
		t.Fatalf("TLSHandshakeTimeout = %s, want 5s", transport.TLSHandshakeTimeout)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Fatalf("IdleConnTimeout = %s, want 90s", transport.IdleConnTimeout)
	}
}

func TestGithubManifestProviderGetManifest(t *testing.T) {
	provider := &GithubManifestProvider{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://github.com/cbowcutt/self-updater/releases/latest/download/manifest.json" {
			t.Fatalf("unexpected URL: %s", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{
				"version": "1.2.3",
				"published_at": "2024-05-06T07:08:09Z",
				"artifacts": {
					"linux-amd64": {"url": "https://example.com/app-linux-amd64", "sha256": "abc123"}
				}
			}`)),
		}, nil
	})}}

	manifest, err := provider.GetManifest()
	if err != nil {
		t.Fatalf("GetManifest returned error: %v", err)
	}
	if manifest.Version != "1.2.3" {
		t.Fatalf("version = %q, want %q", manifest.Version, "1.2.3")
	}
	if got := manifest.Artifacts["linux-amd64"].URL; got != "https://example.com/app-linux-amd64" {
		t.Fatalf("artifact URL = %q, want %q", got, "https://example.com/app-linux-amd64")
	}
}

func TestGithubManifestProviderGetManifestRejectsBadStatus(t *testing.T) {
	provider := &GithubManifestProvider{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("boom")),
		}, nil
	})}}

	if _, err := provider.GetManifest(); err == nil || !strings.Contains(err.Error(), "unexpected status code") {
		t.Fatalf("expected status code error, got %v", err)
	}
}

func TestGithubManifestProviderGetManifestRejectsBadJSON(t *testing.T) {
	provider := &GithubManifestProvider{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
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

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, errors.New("read failed")
}

func TestGithubManifestProviderGetExecutable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bin" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	provider := &GithubManifestProvider{}
	manifest := &Manifest{Artifacts: map[string]models.Artifact{"linux-amd64": {URL: server.URL + "/bin", SHA256: "hash"}}}

	body, err := provider.GetExecutable(manifest, "linux", "amd64")
	if err != nil {
		t.Fatalf("GetExecutable returned error: %v", err)
	}
	if !bytes.Equal(body, []byte("hello")) {
		t.Fatalf("body = %q, want %q", body, "hello")
	}
}

func TestGithubManifestProviderGetExecutableRejectsBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := &GithubManifestProvider{}
	manifest := &Manifest{Artifacts: map[string]models.Artifact{"linux-amd64": {URL: server.URL, SHA256: "hash"}}}

	if _, err := provider.GetExecutable(manifest, "linux", "amd64"); err == nil || !strings.Contains(err.Error(), "unexpected status code") {
		t.Fatalf("expected status code error, got %v", err)
	}
}

func TestGithubManifestProviderGetExecutableReadError(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(errReader{}),
		}, nil
	})
	defer func() { http.DefaultTransport = original }()

	provider := &GithubManifestProvider{}
	manifest := &Manifest{Artifacts: map[string]models.Artifact{"linux-amd64": {URL: "https://example.com/app-linux-amd64", SHA256: "hash"}}}

	_, err := provider.GetExecutable(manifest, "linux", "amd64")
	if err == nil {
		t.Fatal("want read error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read binary") {
		t.Fatalf("error = %v, want read-binary error", err)
	}
}

func TestGithubManifestProviderGetExecutableMissingArtifact(t *testing.T) {
	provider := &GithubManifestProvider{}
	manifest := &Manifest{Artifacts: map[string]models.Artifact{}}

	_, err := provider.GetExecutable(manifest, "linux", "amd64")
	if err == nil {
		t.Fatal("want error when artifact is missing")
	}
}
