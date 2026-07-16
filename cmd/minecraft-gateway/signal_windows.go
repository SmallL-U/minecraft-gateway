//go:build windows

package main

import (
	"fmt"

	"minecraft-gateway/internal/gateway"
	"minecraft-gateway/internal/proc"
)

func signalHandler(gw *gateway.Gateway, configPath string) error {
	for {
		sig, err := proc.WaitForSignals()
		if err != nil {
			return fmt.Errorf("failed to wait for signals: %w", err)
		}

		switch sig {
		case "stop":
			logger.Info("Received stop signal, shutting down...")
			return nil
		case "reload":
			logger.Info("Received reload signal, hot reloading...")
			reloadConfig(gw, configPath)
		}
	}
}
