// Command release builds and signs the update manifest, and verifies one.
//
//	release -dir dist -version 1.4.2 -base-url https://.../v1.4.2 -out dist
//	release verify -manifest dist/manifest.json -sig dist/manifest.json.sig -pubkey keys/public.key
//
// The signing key is read from the RELEASE_SIGNING_KEY environment variable
// (base64 32-byte Ed25519 seed) so it never touches disk or the process list.
package main

import (
	"crypto/ed25519"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	// Replace with the module path from your go.mod.
	"github.com/cbowcutt/self-updater/internal/manifest"
	"github.com/cbowcutt/self-updater/internal/signing"
)

const (
	manifestName = "manifest.json"
	sigName      = "manifest.json.sig"
	keyEnv       = "RELEASE_SIGNING_KEY"
)

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "verify" {
		err = runVerify(os.Args[2:])
	} else {
		err = runSign(os.Args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func runSign(args []string) error {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	dir := fs.String("dir", "dist", "directory containing app-<os>-<arch> binaries")
	version := fs.String("version", "", "release version, semver without leading 'v'")
	baseURL := fs.String("base-url", "", "URL the binaries will be served from")
	out := fs.String("out", "dist", "directory to write manifest.json and manifest.json.sig")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" || *baseURL == "" {
		return fmt.Errorf("-version and -base-url are required")
	}

	priv, err := signing.ParsePrivateKey(os.Getenv(keyEnv))
	if err != nil {
		return fmt.Errorf("%s: %w", keyEnv, err)
	}

	m, err := manifest.Build(*dir, *version, *baseURL, time.Now())
	if err != nil {
		return err
	}
	data, err := m.Marshal()
	if err != nil {
		return err
	}
	sig := signing.Sign(priv, data)

	// Self-check with the derived public key before writing anything out.
	if err := signing.Verify(priv.Public().(ed25519.PublicKey), data, sig); err != nil {
		return fmt.Errorf("self-verification failed: %w", err)
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, manifestName), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, sigName), []byte(sig+"\n"), 0o644); err != nil {
		return err
	}

	fmt.Printf("signed %s v%s with %d artifact(s)\n", manifestName, m.Version, len(m.Artifacts))
	return nil
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "", "path to manifest.json")
	sigPath := fs.String("sig", "", "path to manifest.json.sig")
	pubPath := fs.String("pubkey", "", "path to base64 public key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *manifestPath == "" || *sigPath == "" || *pubPath == "" {
		return fmt.Errorf("-manifest, -sig and -pubkey are required")
	}

	data, err := os.ReadFile(*manifestPath)
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(*sigPath)
	if err != nil {
		return err
	}
	pubRaw, err := os.ReadFile(*pubPath)
	if err != nil {
		return err
	}
	pub, err := signing.ParsePublicKey(string(pubRaw))
	if err != nil {
		return err
	}

	// Authenticate the raw bytes first, then parse.
	if err := signing.Verify(pub, data, string(sig)); err != nil {
		return err
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return err
	}

	fmt.Printf("OK: %s v%s, %d artifact(s)\n", *manifestPath, m.Version, len(m.Artifacts))
	return nil
}
