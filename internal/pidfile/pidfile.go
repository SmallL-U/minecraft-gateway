package pidfile

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// Write writes the current process PID to the specified file.
func Write(path string) error {
	pid := os.Getpid()
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
}

// Read reads the PID from the specified file.
func Read(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %v", err)
	}
	return pid, nil
}

// Remove removes the PID file.
func Remove(path string) error {
	return os.Remove(path)
}

// IsRunning checks if the process with the PID in the file is still running.
func IsRunning(path string) (bool, int, error) {
	pid, err := Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, err
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false, pid, nil
	}

	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return false, pid, nil
	}

	return true, pid, nil
}

// SendSignal sends a signal to the process specified in the PID file.
func SendSignal(path string, sig syscall.Signal) error {
	pid, err := Read(path)
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
