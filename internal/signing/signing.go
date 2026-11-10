// Package signing wraps Ed25519 key handling and detached signatures for
// release artifacts. It operates on plain bytes and knows nothing about what
// is being signed.
//
// Formats: keys and signatures are standard base64. The private key is the
// 32-byte seed (what RELEASE_SIGNING_KEY holds), the public key is the raw
// 32 bytes, and a signature is the raw 64 bytes.
package signing

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// ErrBadSignature is returned when a signature does not match the data.
var ErrBadSignature = errors.New("signing: signature verification failed")

// ParsePrivateKey decodes a base64 32-byte Ed25519 seed into a full private key.
func ParsePrivateKey(b64 string) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("decode signing key: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("signing key must be a %d-byte seed, got %d bytes",
			ed25519.SeedSize, len(seed))
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// ParsePublicKey decodes a base64 32-byte Ed25519 public key.
func ParsePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key must be %d bytes, got %d",
			ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// Sign returns the base64-encoded Ed25519 signature of data.
func Sign(priv ed25519.PrivateKey, data []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data))
}

// Verify checks a base64 signature over data against pub.
func Verify(pub ed25519.PublicKey, data []byte, sigB64 string) error {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrBadSignature
	}
	if !ed25519.Verify(pub, data, sig) {
		return ErrBadSignature
	}
	return nil
}
