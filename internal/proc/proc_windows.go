//go:build windows

package proc

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const (
	eventPrefix    = "Global\\minecraft-gateway"
	eventStop      = eventPrefix + "_stop"
	eventReload    = eventPrefix + "_reload"
	mutexName      = eventPrefix + "_mutex"
)

var (
	mutex        windows.Handle
	stopEvent    windows.Handle
	reloadEvent  windows.Handle
	eventsMu     sync.Mutex
)

// Acquire tries to acquire the process lock using a named mutex.
func Acquire() error {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return fmt.Errorf("failed to convert mutex name: %v", err)
	}

	mutex, err = windows.CreateMutex(nil, false, name)
	if err != nil {
		return fmt.Errorf("failed to create mutex: %v", err)
	}

	event, err := windows.WaitForSingleObject(mutex, 0)
	if err != nil {
		return fmt.Errorf("failed to acquire mutex: %v", err)
	}

	if event == windows.WAIT_TIMEOUT {
		windows.CloseHandle(mutex)
		return fmt.Errorf("another instance is already running")
	}

	// Create stop and reload events
	if err := createEvents(); err != nil {
		windows.ReleaseMutex(mutex)
		windows.CloseHandle(mutex)
		return err
	}

	return nil
}

// Release releases the process lock.
func Release() {
	eventsMu.Lock()
	defer eventsMu.Unlock()

	if stopEvent != 0 {
		windows.CloseHandle(stopEvent)
		stopEvent = 0
	}
	if reloadEvent != 0 {
		windows.CloseHandle(reloadEvent)
		reloadEvent = 0
	}
	if mutex != 0 {
		windows.ReleaseMutex(mutex)
		windows.CloseHandle(mutex)
		mutex = 0
	}
}

// SendReload sends reload signal to the running instance.
func SendReload() error {
	return setEvent(eventReload)
}

// SendStop sends stop signal to the running instance.
func SendStop() error {
	return setEvent(eventStop)
}

// WaitForSignals waits for stop or reload events. Returns "stop", "reload", or error.
func WaitForSignals() (string, error) {
	eventsMu.Lock()
	stop := stopEvent
	reload := reloadEvent
	eventsMu.Unlock()

	if stop == 0 || reload == 0 {
		return "", fmt.Errorf("events not initialized")
	}

	handles := []windows.Handle{stop, reload}
	event, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
	if err != nil {
		return "", fmt.Errorf("failed to wait for events: %v", err)
	}

	switch event {
	case windows.WAIT_OBJECT_0:
		return "stop", nil
	case windows.WAIT_OBJECT_0 + 1:
		// Reset reload event for next use
		windows.ResetEvent(reload)
		return "reload", nil
	default:
		return "", fmt.Errorf("unexpected wait result: %d", event)
	}
}

func createEvents() error {
	var err error

	stopName, _ := windows.UTF16PtrFromString(eventStop)
	stopEvent, err = windows.CreateEvent(nil, true, false, stopName)
	if err != nil {
		return fmt.Errorf("failed to create stop event: %v", err)
	}

	reloadName, _ := windows.UTF16PtrFromString(eventReload)
	reloadEvent, err = windows.CreateEvent(nil, true, false, reloadName)
	if err != nil {
		windows.CloseHandle(stopEvent)
		return fmt.Errorf("failed to create reload event: %v", err)
	}

	return nil
}

func setEvent(eventName string) error {
	name, err := windows.UTF16PtrFromString(eventName)
	if err != nil {
		return fmt.Errorf("failed to convert event name: %v", err)
	}

	handle, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return fmt.Errorf("failed to open event (is the server running?): %v", err)
	}
	defer windows.CloseHandle(handle)

	if err := windows.SetEvent(handle); err != nil {
		return fmt.Errorf("failed to set event: %v", err)
	}

	// Give the server time to handle the event
	time.Sleep(100 * time.Millisecond)
	return nil
}
