package main

import (
	"fmt"

	"github.com/cbowcutt/self-updater/internal/selfUpdater"
)

type SelfUpdater interface {
	IsNewerVersionAvailable() (bool, error)
	CheckAndDownloadLatestVersion() error
}

func main() {
	go func() {
		selfUpdater := selfUpdater.GithubSelfUpdater{}
		err := selfUpdater.CheckAndDownloadLatestVersion()
		if err != nil {
			fmt.Println(err)
		}
	}()
	fmt.Println("Hello, World!")
}
