package main

import (
	"fmt"

	"github.com/cbowcutt/self-updater/internal/binaryProvider"
)

var version = "dev" // overridden at build time; "dev" means a local build

func main() {
	binaryClient := binaryProvider.NewGithubBinaryProvider()
	go func() {
		_, err := binaryClient.CheckAndDownloadLatestVersion(version)
		if err != nil {
			fmt.Println(err)
		}
	}()
	fmt.Println("Hello, World!")
}
