package keys

import _ "embed"

// publicKey contains the trusted Ed25519 key used to authenticate release manifests.
//
//go:embed public.key
var publicKey []byte

// PublicKey returns the encoded trusted public key.
func PublicKey() []byte {
	return append([]byte(nil), publicKey...)
}
