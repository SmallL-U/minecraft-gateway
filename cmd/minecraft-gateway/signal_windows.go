//go:build windows

package main

import (
	"minecraft-gateway/internal/config"
	"minecraft-gateway/internal/logx"
	"minecraft-gateway/internal/proc"
)

func signalHandler(doneChan chan struct{}) {
	for {
		sig, err := proc.WaitForSignals()
		if err != nil {
			logger := logx.GetLogger()
			logger.Errorf("Error waiting for signals: %v", err)
			return
		}

		logger := logx.GetLogger()
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
			newConf, err := config.LoadConfig(configFile)
			if err != nil {
				logger.Errorf("Failed to reload config: %v", err)
				continue
			}
			gw.UpdateConfig(newConf)
			logger.Infof("Configuration reloaded successfully with %d servers", len(newConf.Servers))
		}
	}
}
