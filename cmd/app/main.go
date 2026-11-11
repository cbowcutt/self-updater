package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/cbowcutt/self-updater/internal/binaryProvider"
	"github.com/cbowcutt/self-updater/internal/selfUpdater"
)

var version = "dev" // overridden at build time; "dev" means a local build

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("self-updater starting", "version", version, "platform", runtime.GOOS+"-"+runtime.GOARCH)
	executablePath, err := os.Executable()
	if err != nil {
		slog.Error("failed to locate current executable", "error", err)
		return
	}
	executablePath, err = filepath.Abs(executablePath)
	if err != nil {
		slog.Error("failed to resolve current executable path", "error", err)
		return
	}
	executablePath, err = filepath.EvalSymlinks(executablePath)
	if err != nil {
		slog.Error("failed to resolve current executable symlinks", "path", executablePath, "error", err)
		return
	}

	shutdownContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	binaryClient := binaryProvider.NewGithubBinaryProvider()

	restartChan := make(chan struct{}, 1)
	if version != "dev" {
		go func() {
			ticker := time.NewTicker(time.Duration(5 * time.Second))
			defer ticker.Stop()

			for {
				select {
				case <-shutdownContext.Done():
					return
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
					if shutdownContext.Err() != nil {
						return
					}
					slog.Info("applying downloaded update")
					if err := selfUpdater.Update(binaryBytes); err != nil {
						slog.Error("update installation failed", "error", err)
						continue
					}
					slog.Info("update installation completed")
					select {
					case restartChan <- struct{}{}:
					case <-shutdownContext.Done():
					}
					return
				}
			}
		}()
	} else {
		slog.Info("development version detected, skipping update checks")
	}

	statusTicker := time.NewTicker(time.Second)
	defer statusTicker.Stop()
	for {
		select {
		case <-shutdownContext.Done():
			return
		case <-restartChan:
			if shutdownContext.Err() != nil {
				return
			}
			if err := restart(executablePath); err != nil {
				slog.Error("failed to restart application", "error", err)
			}
			return
		case <-statusTicker.C:
			slog.Info("application running")
		}
	}
}
