package models

import (
	"encoding/json"
	"time"
)

// Artifact describes one downloadable binary.
type Artifact struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"` // lowercase hex
}

// Manifest describes the latest release. Artifacts is keyed by
// "<GOOS>-<GOARCH>", e.g. "linux-amd64" or "windows-amd64".
type Manifest struct {
	Version     string              `json:"version"` // semver, no leading "v"
	PublishedAt time.Time           `json:"published_at"`
	Artifacts   map[string]Artifact `json:"artifacts"`
}

// Marshal returns the canonical bytes that get signed and published.
// encoding/json sorts map keys, so output is deterministic.
func (m *Manifest) Marshal() ([]byte, error) {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
