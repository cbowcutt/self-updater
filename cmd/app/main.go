package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/cbowcutt/self-updater/internal/binaryProvider"
	"github.com/cbowcutt/self-updater/internal/selfUpdater"
)

// overridden at build time; "dev" means a local build
var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("self-updater starting", "version", version, "platform", runtime.GOOS+"-"+runtime.GOARCH)
	binaryClient := binaryProvider.NewGithubBinaryProvider()

	doneChan := make(chan bool, 1)
	restartChan := make(chan struct{}, 1)
	go func(done chan bool) {
		ticker := time.NewTicker(time.Duration(1 * time.Second))

		for {
			select {
			case <-ticker.C:
				shouldUpdate, binaryBytes, err := binaryClient.CheckAndDownloadLatestVersion(version)
				if err != nil {
					slog.Error("update check failed", "error", err)
					continue
				}
				if !shouldUpdate {
					slog.Info("application is up to date")
					continue
				}
				slog.Info("applying downloaded update")
				if err := selfUpdater.Update(binaryBytes); err != nil {
					slog.Error("update installation failed", "error", err)
					continue
				}
				slog.Info("update installation completed")
				restartChan <- struct{}{}
				return
			case <-done:
				ticker.Stop()
				return
			}
		}
	}(doneChan)
	shutdownContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	for {
		select {
		case <-shutdownContext.Done():
			doneChan <- true
			return
		case <-restartChan:
			if err := restart(); err != nil {
				slog.Error("failed to restart application", "error", err)
			}
			return
		default:
			time.Sleep(time.Duration(1 * time.Second))
			slog.Info("application running")
		}
	}
}

func restart() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}

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
