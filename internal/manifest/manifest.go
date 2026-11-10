// Package manifest defines the signed update manifest shared by the release
// tool (which produces it) and the updater client (which verifies it).
//
// Trust model: the signature (see package signing) covers the exact bytes of
// manifest.json. Clients must verify those raw bytes BEFORE parsing them, so
// nothing attacker-controlled is interpreted until it has been authenticated.
package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/cbowcutt/self-updater/internal/manifest/models"
)

// Manifest is the signed release manifest model used by clients and publishers.
type Manifest = models.Manifest

// Artifact describes one downloadable release binary.
type Artifact = models.Artifact

var (
	semverRe = regexp.MustCompile(
		`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

	// Release binaries are named app-<os>-<arch>[.exe]. Anything else in the
	// directory (manifest.json, signatures, checksums) is ignored.
	artifactRe = regexp.MustCompile(`^app-([a-z0-9]+)-([a-z0-9]+)(\.exe)?$`)
)

// Build hashes every release binary in dir and returns a manifest describing them.
// baseURL is the base URL the published files will be served from.
func Build(dir, version, baseURL string, now time.Time) (*Manifest, error) {
	if !semverRe.MatchString(version) {
		return nil, fmt.Errorf("invalid version %q: want semver without a leading 'v'", version)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	m := &Manifest{
		Version:     version,
		PublishedAt: now.UTC().Truncate(time.Second),
		Artifacts:   make(map[string]Artifact),
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		match := artifactRe.FindStringSubmatch(e.Name())
		if match == nil {
			continue
		}
		platform := match[1] + "-" + match[2]
		if _, dup := m.Artifacts[platform]; dup {
			return nil, fmt.Errorf("multiple binaries for platform %s", platform)
		}

		sum, err := hashFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		u, err := url.JoinPath(baseURL, e.Name())
		if err != nil {
			return nil, fmt.Errorf("build url for %s: %w", e.Name(), err)
		}
		m.Artifacts[platform] = Artifact{URL: u, SHA256: sum}
	}

	if len(m.Artifacts) == 0 {
		return nil, fmt.Errorf("no app-<os>-<arch> binaries found in %s", dir)
	}
	return m, nil
}

// Parse decodes and validates manifest bytes. Call Verify on the same bytes
// first; Parse does not authenticate anything.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if !semverRe.MatchString(m.Version) {
		return nil, fmt.Errorf("manifest has invalid version %q", m.Version)
	}
	if len(m.Artifacts) == 0 {
		return nil, errors.New("manifest has no artifacts")
	}
	return &m, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
