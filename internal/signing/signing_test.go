package signing

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
)

func newKeys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func TestSignVerify(t *testing.T) {
	pub, priv := newKeys(t)
	data := []byte(`{"version":"1.0.0"}`)
	sig := Sign(priv, data)

	if err := Verify(pub, data, sig); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := Verify(pub, data, sig+"\n"); err != nil {
		t.Errorf("trailing newline in sig file should be tolerated: %v", err)
	}
}

func TestVerifyRejects(t *testing.T) {
	pub, priv := newKeys(t)
	otherPub, _ := newKeys(t)
	data := []byte(`{"version":"1.0.0"}`)
	sig := Sign(priv, data)

	tests := []struct {
		name string
		pub  ed25519.PublicKey
		data []byte
		sig  string
	}{
		{"tampered data", pub, []byte(`{"version":"9.9.9"}`), sig},
		{"wrong key", otherPub, data, sig},
		{"empty sig", pub, data, ""},
		{"garbage sig", pub, data, "!!!not-base64!!!"},
		{"short sig", pub, data, base64.StdEncoding.EncodeToString([]byte("short"))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Verify(tc.pub, tc.data, tc.sig)
			if !errors.Is(err, ErrBadSignature) {
				t.Fatalf("err = %v, want ErrBadSignature", err)
			}
		})
	}
}

func TestParsePrivateKey(t *testing.T) {
	seed := bytes.Repeat([]byte{7}, ed25519.SeedSize)
	b64 := base64.StdEncoding.EncodeToString(seed)

	priv, err := ParsePrivateKey(b64 + "\n") // secrets often carry a newline
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(priv.Seed(), seed) {
		t.Error("seed did not round trip")
	}

	// A full 64-byte private key is a common mistake; it must be rejected.
	full := base64.StdEncoding.EncodeToString(ed25519.NewKeyFromSeed(seed))
	for name, in := range map[string]string{
		"empty":      "",
		"not base64": "%%%",
		"64 bytes":   full,
	} {
		if _, err := ParsePrivateKey(in); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestParsePublicKey(t *testing.T) {
	pub, _ := newKeys(t)
	got, err := ParsePublicKey(base64.StdEncoding.EncodeToString(pub) + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pub) {
		t.Error("public key did not round trip")
	}
	if _, err := ParsePublicKey("AAAA"); err == nil {
		t.Error("want error for wrong-length key")
	}
}
