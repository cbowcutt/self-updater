package main

import (
	"fmt"

	"github.com/cbowcutt/self-updater/internal/manifest/models"
	"github.com/cbowcutt/self-updater/internal/manifest/provider"
	"github.com/cbowcutt/self-updater/internal/selfUpdater"
)

var version = "dev" // overridden at build time; "dev" means a local build

// SelfUpdater exposes the minimal contract needed by the app to check for updates.
type SelfUpdater interface {
	IsNewerVersionAvailable() (bool, error)
	GetManifest() (*models.Manifest, error)
}

func main() {
	manifestProvider := provider.NewManifestProvider()
	selfUpdater := selfUpdater.NewGithubSelfUpdater(manifestProvider)
	go func() {
		err := selfUpdater.CheckAndDownloadLatestVersion(version)
		if err != nil {
			fmt.Println(err)
		}
	}()
	fmt.Println("Hello, World!")
}
