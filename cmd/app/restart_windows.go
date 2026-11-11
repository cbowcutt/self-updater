//go:build windows

package main

import (
	"os"

	"github.com/cbowcutt/self-updater/internal/selfUpdater"
)

// restart lets the already-running helper replace this executable after this
// process exits, then relaunch the updated application.
func restart(_ string) error {
	return nil
}

func runUpdateHelper(args []string) bool {
	if len(args) == 0 || args[0] != "--self-updater-helper" {
		return false
	}
	if err := selfUpdater.RunWindowsUpdateHelper(args[1:]); err != nil {
		_, _ = os.Stderr.WriteString("self-update helper failed: " + err.Error() + "\n")
		os.Exit(1)
	}
	return true
}
