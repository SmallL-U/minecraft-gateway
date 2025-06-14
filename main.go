package main

import (
	"minecraft-gateway/config"
	"minecraft-gateway/gateway"
	"minecraft-gateway/logx"
	"minecraft-gateway/util"
	"minecraft-gateway/whitelist"
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
	whitelistFile := "whitelist.txt"
	// save default config if it does not exist
	if fileNotExists(configFile) {
		logger.Info("config file not found, creating default config...")
		defaultConfig := config.Default()
		if err := util.SaveJSON(configFile, defaultConfig); err != nil {
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
	// save default whitelist
	if fileNotExists(whitelistFile) {
		logger.Info("whitelist config file not found, creating default whitelist...")
		defaultWhitelist := whitelist.Default()
		if err := util.SaveLines(whitelistFile, defaultWhitelist.ToLines()); err != nil {
			logger.Fatalf("Failed to save default whitelist: %v", err)
		}
		logger.Info("Default whitelist created successfully.")
	}
	// load whitelist
	lines, err := util.ReadLines(whitelistFile)
	if err != nil {
		logger.Fatalf("Failed to read whitelist file: %v", err)
	}
	nets := whitelist.ParseLines(lines)
	allowlist := whitelist.New(nets)
	logger.Infof("Loaded whitelist with %d entries", len(nets))
	// new instance of gateway
	gw = gateway.NewGateway(conf, allowlist)
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
				logger.Info("Received SIGHUP signal, hot reloading...")
				// load config
				newConf, err := config.LoadConfig(configFile)
				if err != nil {
					logger.Errorf("Failed to reload config: %v", err)
					continue
				}
				gw.UpdateConfig(newConf)
				logger.Infof("Configuration reloaded successfully: %+v", newConf)
				// load whitelist
				lines, err := util.ReadLines(whitelistFile)
				if err != nil {
					logger.Errorf("Failed to read whitelist file: %v", err)
					continue
				}
				nets := whitelist.ParseLines(lines)
				allowlist := whitelist.New(nets)
				gw.UpdateWhitelist(allowlist)
				logger.Infof("Whitelist reloaded successfully with %d entries", len(nets))
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
