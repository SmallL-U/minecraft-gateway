//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"minecraft-gateway/internal/gateway"
)

func signalHandler(gw *gateway.Gateway, configPath string) error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigChan)

	for sig := range sigChan {
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			logger.Info("Received termination signal, shutting down...")
			return nil
		case syscall.SIGHUP:
			logger.Info("Received SIGHUP signal, hot reloading...")
			reloadConfig(gw, configPath)
		default:
			logger.Warnf("Received unknown signal: %v", sig)
		}
	}
	return nil
}
