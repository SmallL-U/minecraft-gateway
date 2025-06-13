package main

import (
	"go.uber.org/zap"
	"minecraft-gateway/config"
	"os"
)

var logger = func() *zap.SugaredLogger {
	production, _ := zap.NewProduction()
	return production.Sugar()
}()

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
	loadedConfig, err := config.LoadConfig(configFile)
	if err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}
	logger.Infof("Loaded config: %+v", loadedConfig)
}
