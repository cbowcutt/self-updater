package selfUpdater

import (
	"fmt"
	"log/slog"
	"os"
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

	stat, err := os.Lstat(cmdPath)
	if err != nil {
		slog.Error("failed to inspect current executable", "path", cmdPath, "error", err)
		return fmt.Errorf("failed to stat executable: %w", err)
	}
	if stat.Mode()&os.ModeSymlink != 0 {
		slog.Debug("current executable is a symlink", "path", cmdPath)
		cmdPath, err = os.Readlink(cmdPath)
		if err != nil {
			slog.Error("failed to resolve executable symlink", "error", err)
			return fmt.Errorf("failed to resolve executable symlink: %w", err)
		}
		slog.Debug("resolved executable symlink", "target", cmdPath)
	}

	if err := os.WriteFile(cmdPath, *binaryBytes, stat.Mode()); err != nil {
		slog.Error("failed to replace executable with update", "path", cmdPath, "error", err)
		return fmt.Errorf("failed to write executable: %w", err)
	}

	slog.Info("self-update binary written successfully", "path", cmdPath, "bytes", len(*binaryBytes))
	return nil
}
