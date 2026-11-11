//go:build !windows

package selfUpdater

import "fmt"

func launchWindowsUpdateHelper(string, string, []string) error {
	return fmt.Errorf("Windows update helper is unavailable on this platform")
}
