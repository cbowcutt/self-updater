//go:build linux

package main

import (
	"os"
	"syscall"
)

// restart replaces this process with the updated executable. Keeping the same
// process preserves the shell's foreground-process and signal relationship.
func restart(executable string) error {
	return syscall.Exec(executable, os.Args, os.Environ())
}

func runUpdateHelper([]string) bool {
	return false
}
