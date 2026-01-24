package main

import (
	"os"
	"os/signal"
	"syscall"

	"minecraft-gateway/internal/config"
	"minecraft-gateway/internal/gateway"
	"minecraft-gateway/internal/logx"
	"minecraft-gateway/internal/pidfile"
	"minecraft-gateway/internal/util"
	"minecraft-gateway/internal/whitelist"
)

const (
	configFile    = "config.yml"
	whitelistFile = "whitelist.txt"
	pidFile       = "minecraft-gateway.pid"
)

var gw *gateway.Gateway
var logger = logx.GetLogger()

func fileNotExists(filename string) bool {
	_, err := os.Stat(filename)
	return os.IsNotExist(err)
}

func handleReload() {
	if err := pidfile.SendSignal(pidFile, syscall.SIGHUP); err != nil {
		logger.Fatalf("Failed to send reload signal: %v", err)
	}
	logger.Info("Reload signal sent successfully")
}

func handleStop() {
	if err := pidfile.SendSignal(pidFile, syscall.SIGTERM); err != nil {
		logger.Fatalf("Failed to send stop signal: %v", err)
	}
	logger.Info("Stop signal sent successfully")
}

func runServer() {
	defer func() {
		_ = logger.Sync()
	}()

	// Check if another instance is already running
	running, pid, err := pidfile.IsRunning(pidFile)
	if err != nil {
		logger.Fatalf("Failed to check PID file: %v", err)
	}
	if running {
		logger.Fatalf("Another instance is already running (PID: %d)", pid)
	}

	// Write PID file
	if err := pidfile.Write(pidFile); err != nil {
		logger.Fatalf("Failed to write PID file: %v", err)
	}
	defer func() {
		_ = pidfile.Remove(pidFile)
	}()

	// Save default config if it does not exist
	if fileNotExists(configFile) {
		logger.Info("config file not found, creating default config...")
		defaultConfig := config.Default()
		if err := util.SaveYAML(configFile, defaultConfig); err != nil {
			logger.Fatalf("Failed to save default config: %v", err)
		}
		logger.Infof("Default config created successfully. Please modify %s and restart the application.", configFile)
		return
	}

	// Load config
	conf, err := config.LoadConfig(configFile)
	if err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}
	logger.Infof("Loaded config: %+v", conf)

	// Save default whitelist
	if fileNotExists(whitelistFile) {
		logger.Info("whitelist config file not found, creating default whitelist...")
		defaultWhitelist := whitelist.Default()
		if err := util.SaveLines(whitelistFile, defaultWhitelist.ToLines()); err != nil {
			logger.Fatalf("Failed to save default whitelist: %v", err)
		}
		logger.Info("Default whitelist created successfully.")
	}

	// Load whitelist
	lines, err := util.ReadLines(whitelistFile)
	if err != nil {
		logger.Fatalf("Failed to read whitelist file: %v", err)
	}
	nets := whitelist.ParseLines(lines)
	allowlist := whitelist.New(nets)
	logger.Infof("Loaded whitelist with %d entries", len(nets))

	// New instance of gateway
	gw = gateway.NewGateway(conf, allowlist)
	logger.Infof("Created new minecraft gateway")

	// Create channels
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	errChan := make(chan error, 1)
	doneChan := make(chan struct{})

	// Start the gateway in a goroutine
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
				// Load config
				newConf, err := config.LoadConfig(configFile)
				if err != nil {
					logger.Errorf("Failed to reload config: %v", err)
					continue
				}
				gw.UpdateConfig(newConf)
				logger.Infof("Configuration reloaded successfully: %+v", newConf)
				// Load whitelist
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
