//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"minecraft-gateway/internal/gateway"
)

func signalHandler(gw *gateway.Gateway, configPath string, doneChan chan struct{}) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range sigChan {
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			logger.Info("Received termination signal, shutting down...")
			if err := gw.Stop(); err != nil {
				logger.Warnf("Failed to shut down gateway: %s", err)
			}
			close(doneChan)
			return
		case syscall.SIGHUP:
			logger.Info("Received SIGHUP signal, hot reloading...")
			reloadConfig(gw, configPath)
		default:
			logger.Warnf("Received unknown signal: %v", sig)
		}
	}
}
