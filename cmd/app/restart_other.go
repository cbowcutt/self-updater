//go:build !linux && !windows

package main

import (
	"log/slog"
	"os"
	"os/exec"
)

// restart starts the replacement process on platforms without exec-style
// process replacement, then releases the child handle before exiting.
func restart(executable string) error {
	command := exec.Command(executable, os.Args[1:]...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = os.Environ()

	if err := command.Start(); err != nil {
		return err
	}
	if err := command.Process.Release(); err != nil {
		slog.Warn("failed to release restarted process handle", "error", err)
	}
	return nil
}

func runUpdateHelper([]string) bool {
	return false
}
