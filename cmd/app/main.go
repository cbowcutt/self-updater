package main

import (
	"context"
	"log/slog"
	"os"
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

	doneChan := make(chan bool)
	go func(done chan bool) {
		ticker := time.NewTicker(time.Duration(1 * time.Minute))

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
		default:
			time.Sleep(time.Duration(1 * time.Second))
			slog.Info("application running")
		}
	}
}
