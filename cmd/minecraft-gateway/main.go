package main

import (
	"os"

	"minecraft-gateway/internal/config"
	"minecraft-gateway/internal/gateway"
	"minecraft-gateway/internal/logx"
	"minecraft-gateway/internal/proc"
)

const configFile = "config.yml"

var gw *gateway.Gateway
var logger = logx.GetLogger()

func handleReload() {
	if err := proc.SendReload(); err != nil {
		logger.Fatalf("Failed to send reload signal: %v", err)
	}
	logger.Info("Reload signal sent successfully")
}

func handleStop() {
	if err := proc.SendStop(); err != nil {
		logger.Fatalf("Failed to send stop signal: %v", err)
	}
	logger.Info("Stop signal sent successfully")
}

func runServer() {
	defer func() {
		_ = logger.Sync()
	}()

	// Acquire process lock
	if err := proc.Acquire(); err != nil {
		logger.Fatalf("Failed to acquire process lock: %v", err)
	}
	defer proc.Release()

	// Load config
	conf, err := config.LoadConfig(configFile)
	if err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}
	logger.Infof("Loaded config with %d servers", len(conf.Servers))

	// New instance of gateway
	gw = gateway.NewGateway(conf)
	logger.Info("Created new minecraft gateway")

	errChan := make(chan error, 1)
	doneChan := make(chan struct{})

	// Start the gateway in a goroutine
	go func() {
		if err := gw.Start(); err != nil {
			errChan <- err
		}
	}()

	// Start signal handler
	go signalHandler(doneChan)

	select {
	case err := <-errChan:
		logger.Fatalf("Failed to start gateway: %v", err)
	case <-doneChan:
		logger.Info("Gateway shutdown gracefully.")
	}
}

func printUsage() {
	logger.Info("Usage: minecraft-gateway [command]")
	logger.Info("Commands:")
	logger.Info("  (none)    Start the gateway server")
	logger.Info("  reload    Reload configuration (send SIGHUP to running instance)")
	logger.Info("  stop      Stop the running instance (send SIGTERM)")
}

func main() {
	defer func() {
		_ = logger.Sync()
	}()

	if len(os.Args) < 2 {
		runServer()
		return
	}

	switch os.Args[1] {
	case "reload":
		handleReload()
	case "stop":
		handleStop()
	case "help", "-h", "--help":
		printUsage()
	default:
		logger.Errorf("Unknown command: %s", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}
