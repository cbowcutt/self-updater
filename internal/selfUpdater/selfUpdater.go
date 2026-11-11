package selfUpdater

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type SelfUpdater struct {
}

func Update(binaryBytes *[]byte) error {
	if binaryBytes == nil {
		err := fmt.Errorf("update binary is nil")
		slog.Error("cannot apply update", "error", err)
		return err
	}
	slog.Info("starting self-update", "binary_bytes", len(*binaryBytes), "platform", runtime.GOOS+"-"+runtime.GOARCH)

	cmdPath, err := os.Executable()
	if err != nil {
		slog.Error("failed to locate current executable", "error", err)
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	slog.Debug("located current executable", "path", cmdPath)

	if runtime.GOOS == "windows" && !strings.HasSuffix(cmdPath, ".exe") {
		cmdPath = cmdPath + ".exe"
	}

	cmdPath, err = filepath.Abs(cmdPath)
	if err != nil {
		slog.Error("failed to resolve current executable path", "path", cmdPath, "error", err)
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}
	cmdPath, err = filepath.EvalSymlinks(cmdPath)
	if err != nil {
		slog.Error("failed to resolve executable symlinks", "path", cmdPath, "error", err)
		return fmt.Errorf("failed to resolve executable symlinks: %w", err)
	}

	stat, err := os.Stat(cmdPath)
	if err != nil {
		slog.Error("failed to inspect current executable", "path", cmdPath, "error", err)
		return fmt.Errorf("failed to stat executable: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(cmdPath), "."+filepath.Base(cmdPath)+".update-*")
	if err != nil {
		slog.Error("failed to create temporary update file", "path", cmdPath, "error", err)
		return fmt.Errorf("failed to create temporary update file: %w", err)
	}
	tempPath := tempFile.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	defer tempFile.Close()

	if _, err := tempFile.Write(*binaryBytes); err != nil {
		slog.Error("failed to write temporary update file", "path", tempPath, "error", err)
		return fmt.Errorf("failed to write temporary update file: %w", err)
	}
	if err := tempFile.Chmod(stat.Mode().Perm()); err != nil {
		slog.Error("failed to set update file permissions", "path", tempPath, "error", err)
		return fmt.Errorf("failed to set update file permissions: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		slog.Error("failed to sync temporary update file", "path", tempPath, "error", err)
		return fmt.Errorf("failed to sync temporary update file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		slog.Error("failed to close temporary update file", "path", tempPath, "error", err)
		return fmt.Errorf("failed to close temporary update file: %w", err)
	}
	if runtime.GOOS == "windows" {
		if err := launchWindowsUpdateHelper(cmdPath, tempPath, os.Args[1:]); err != nil {
			slog.Error("failed to start Windows update helper", "error", err)
			return fmt.Errorf("failed to start Windows update helper: %w", err)
		}
		removeTemp = false
		slog.Info("Windows update staged; helper will replace executable after shutdown", "path", cmdPath, "staged_path", tempPath)
		return nil
	}
	if err := os.Rename(tempPath, cmdPath); err != nil {
		slog.Error("failed to atomically replace executable", "path", cmdPath, "error", err)
		return fmt.Errorf("failed to replace executable: %w", err)
	}

	slog.Info("self-update binary written successfully", "path", cmdPath, "bytes", len(*binaryBytes))
	return nil
}
