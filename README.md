# self-updater

A Go self-updating application that checks GitHub's latest-release manifest, downloads the matching platform binary, verifies its SHA-256 checksum, installs it, and restarts into the new version.

## How it works

1. The release client downloads `manifest.json` from GitHub's `/releases/latest/download/` URL and compares its version with the running version.
2. If the manifest is newer, it downloads the artifact for the current OS and architecture and verifies the binary's SHA-256 checksum against the manifest.
3. It stages the update beside the running executable and replaces it. On Linux, the process is replaced in-place so terminal signals continue to reach the running application.
    On Windows, it starts a helper copy of the app, exits the running process, and lets the helper replace and relaunch the executable. The install directory must be writable by the current user; per-user install locations are recommended.

The manifest is generated and signed by the release workflow. Before parsing a manifest or using its artifact URLs and checksums, the client verifies its detached Ed25519 signature against the trusted public key embedded from [keys/public.key](keys/public.key). Artifact checksums are then checked after download. Updating the trusted signing key requires building and distributing an updater containing the new public key.

## Requirements

- Go version specified in [go.mod](go.mod) or later.
- Network access to the GitHub API and release asset URLs.

## Build and run

Build a local development binary:

```sh
go build -o app ./cmd/app
./app
```

A local build identifies itself as `dev` and skips automatic update checks. Release builds get their version embedded by the GitHub Actions workflow. To build manually with a version:

```sh
go build -ldflags "-X main.version=0.1.0" -o app ./cmd/app
```

Run the test suite:

```sh
go test ./...
```

## Release process

The GitHub Actions workflow in `.github/workflows/release.yml` runs when a version tag matching `v*.*.*` is pushed. It tests the project, builds Linux, macOS, and Windows binaries, generates `manifest.json` and `manifest.json.sig`, verifies the signature, and publishes the release.

Configure the repository secret `RELEASE_SIGNING_KEY` with a base64-encoded 32-byte Ed25519 seed before publishing. Keep the private signing key secret. The corresponding base64-encoded public key belongs in `keys/public.key` and is used by the release workflow to verify the generated signature.

The release manifest records each binary's download URL and SHA-256 checksum. Binaries must use the expected naming format:

- `app-<os>-<arch>` on Unix-like systems
- `app-<os>-<arch>.exe` on Windows

For example: `app-linux-amd64` and `app-windows-amd64.exe`.

## Release utility

The `cmd/release` utility creates and signs a manifest from a directory of binaries. It reads the private key from `RELEASE_SIGNING_KEY`:

```sh
RELEASE_SIGNING_KEY="<base64-ed25519-seed>" go run ./cmd/release \
  -dir dist \
  -version 0.1.0 \
  -base-url https://github.com/cbowcutt/self-updater/releases/download/v0.1.0 \
  -out dist
```

Verify the generated files using the repository public key:

```sh
go run ./cmd/release verify \
  -manifest dist/manifest.json \
  -sig dist/manifest.json.sig \
  -pubkey keys/public.key
```

