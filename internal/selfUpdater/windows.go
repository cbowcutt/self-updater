//go:build windows

package selfUpdater

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const windowsUpdateHelperArg = "--self-updater-helper"
const windowsCreateNoWindow = 0x08000000

func launchWindowsUpdateHelper(executable, stagedPath string, appArgs []string) error {
	runningExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate helper source executable: %w", err)
	}
	helper, err := os.CreateTemp("", "self-updater-helper-*.exe")
	if err != nil {
		return fmt.Errorf("create helper executable: %w", err)
	}
	helperPath := helper.Name()
	defer helper.Close()
	removeHelper := true
	defer func() {
		if removeHelper {
			_ = os.Remove(helperPath)
		}
	}()

	source, err := os.Open(runningExecutable)
	if err != nil {
		return fmt.Errorf("open helper source executable: %w", err)
	}
	_, copyErr := io.Copy(helper, source)
	closeErr := source.Close()
	if copyErr != nil {
		return fmt.Errorf("copy helper executable: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close helper source executable: %w", closeErr)
	}
	if err := helper.Sync(); err != nil {
		return fmt.Errorf("sync helper executable: %w", err)
	}
	if err := helper.Close(); err != nil {
		return fmt.Errorf("close helper executable: %w", err)
	}

	args := []string{
		windowsUpdateHelperArg,
		strconv.Itoa(os.Getpid()),
		executable,
		stagedPath,
		"--",
	}
	args = append(args, appArgs...)
	command := exec.Command(helperPath, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windowsCreateNoWindow}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start helper process: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release helper process: %w", err)
	}
	removeHelper = false
	return nil
}

// RunWindowsUpdateHelper waits for the application process to exit, installs
// the staged executable, and starts the updated application.
func RunWindowsUpdateHelper(args []string) error {
	if len(args) < 4 || args[3] != "--" {
		return fmt.Errorf("invalid Windows update helper arguments")
	}
	parentPID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		return fmt.Errorf("invalid parent process ID: %w", err)
	}
	executable := args[1]
	stagedPath := args[2]
	appArgs := args[4:]

	process, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(parentPID))
	if err != nil {
		if err != syscall.Errno(87) { // ERROR_INVALID_PARAMETER: parent already exited
			return fmt.Errorf("open parent process: %w", err)
		}
	} else {
		defer syscall.CloseHandle(process)
		result, err := syscall.WaitForSingleObject(process, syscall.INFINITE)
		if err != nil {
			return fmt.Errorf("wait for parent process: %w", err)
		}
		if result != syscall.WAIT_OBJECT_0 {
			return fmt.Errorf("wait for parent process returned unexpected status %#x", result)
		}
	}

	var replaceErr error
	for attempt := 0; attempt < 20; attempt++ {
		replaceErr = os.Rename(stagedPath, executable)
		if replaceErr == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if replaceErr != nil {
		if startErr := startUpdatedApplication(executable, appArgs); startErr != nil {
			return fmt.Errorf("replace executable %q: %w (also failed to restart existing application: %v)", filepath.Base(executable), replaceErr, startErr)
		}
		return fmt.Errorf("replace executable %q after retries: %w; restarted the existing version", filepath.Base(executable), replaceErr)
	}
	if err := startUpdatedApplication(executable, appArgs); err != nil {
		return fmt.Errorf("start updated application: %w", err)
	}
	return nil
}

func startUpdatedApplication(executable string, appArgs []string) error {
	command := exec.Command(executable, appArgs...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return err
	}
	if err := command.Process.Release(); err != nil {
		return err
	}
	return nil
}
