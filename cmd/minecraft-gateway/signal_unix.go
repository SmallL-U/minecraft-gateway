//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"minecraft-gateway/internal/config"
	"minecraft-gateway/internal/logx"
)

func signalHandler(doneChan chan struct{}) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range sigChan {
		logger := logx.GetLogger()
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
			newConf, err := config.LoadConfig(configFile)
			if err != nil {
				logger.Errorf("Failed to reload config: %v", err)
				continue
			}
			gw.UpdateConfig(newConf)
			logger.Infof("Configuration reloaded successfully with %d servers", len(newConf.Servers))
		default:
			logger.Warnf("Received unknown signal: %v", sig)
		}
	}
}
