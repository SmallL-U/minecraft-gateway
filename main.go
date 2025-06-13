package main

import (
	"minecraft-gateway/config"
	"minecraft-gateway/gateway"
	"minecraft-gateway/logx"
	"os"
	"os/signal"
	"syscall"
)

var gw *gateway.Gateway
var logger = logx.GetLogger()

func fileNotExists(filename string) bool {
	_, err := os.Stat(filename)
	return os.IsNotExist(err)
}

func main() {
	defer func() {
		_ = logger.Sync()
	}()
	configFile := "config.json"
	// save default config if it does not exist
	if fileNotExists(configFile) {
		logger.Info("Config file not found, creating default config...")
		defaultConfig := config.DefaultConfig()
		if err := defaultConfig.Save(configFile); err != nil {
			logger.Fatalf("Failed to save default config: %v", err)
		}
		logger.Info("Default config created successfully. Please modify %s and restart the application.", configFile)
		return
	}
	// load config
	conf, err := config.LoadConfig(configFile)
	if err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}
	logger.Infof("Loaded config: %+v", conf)
	// new instance of gateway
	gw = gateway.NewGateway(conf)
	logger.Infof("Created new minecraft gateway")
	// create channels
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	errChan := make(chan error, 1)
	doneChan := make(chan struct{})
	// start the gateway in a goroutine
	go func() {
		if err := gw.Start(); err != nil {
			errChan <- err
		}
	}()

	go func() {
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
				logger.Info("Received SIGHUP signal, reloading configuration...")
				// TODO: reload configuration logic can be added here
			default:
				logger.Warnf("Received unknown signal: %v", sig)
			}
		}
	}()

	select {
	case err := <-errChan:
		logger.Fatalf("Failed to start gateway: %v", err)
	case <-doneChan:
		logger.Info("Gateway shutdown gracefully.")
	}
}
