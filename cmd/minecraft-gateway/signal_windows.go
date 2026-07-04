//go:build windows

package main

import (
	"minecraft-gateway/internal/gateway"
	"minecraft-gateway/internal/proc"
)

func signalHandler(gw *gateway.Gateway, configPath string, doneChan chan struct{}) {
	for {
		sig, err := proc.WaitForSignals()
		if err != nil {
			logger.Errorf("Error waiting for signals: %v", err)
			return
		}

		switch sig {
		case "stop":
			logger.Info("Received stop signal, shutting down...")
			if err := gw.Stop(); err != nil {
				logger.Warnf("Failed to shut down gateway: %s", err)
			}
			close(doneChan)
			return
		case "reload":
			logger.Info("Received reload signal, hot reloading...")
			reloadConfig(gw, configPath)
		}
	}
}
