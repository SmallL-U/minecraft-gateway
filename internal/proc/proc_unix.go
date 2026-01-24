//go:build !windows

package proc

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const pidFile = "/tmp/minecraft-gateway.pid"

// Acquire tries to acquire the process lock. Returns error if another instance is running.
func Acquire() error {
	if running, pid := isRunning(); running {
		return fmt.Errorf("another instance is already running (PID: %d)", pid)
	}
	return writePID()
}

// Release releases the process lock.
func Release() {
	_ = os.Remove(pidFile)
}

// SendReload sends reload signal to the running instance.
func SendReload() error {
	return sendSignal(syscall.SIGHUP)
}

// SendStop sends stop signal to the running instance.
func SendStop() error {
	return sendSignal(syscall.SIGTERM)
}

func writePID() error {
	pid := os.Getpid()
	return os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)
}

func readPID() (int, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %v", err)
	}
	return pid, nil
}

func isRunning() (bool, int) {
	pid, err := readPID()
	if err != nil {
		return false, 0
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false, pid
	}

	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return false, pid
	}

	return true, pid
}

func sendSignal(sig syscall.Signal) error {
	pid, err := readPID()
	if err != nil {
		return fmt.Errorf("failed to read PID file: %v", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %v", pid, err)
	}

	if err := process.Signal(sig); err != nil {
		return fmt.Errorf("failed to send signal to process %d: %v", pid, err)
	}

	return nil
}
