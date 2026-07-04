package main

import (
	"flag"
	"fmt"
	"os"

	"minecraft-gateway/internal/config"
	"minecraft-gateway/internal/gateway"
	"minecraft-gateway/internal/logx"
	"minecraft-gateway/internal/proc"
)

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

func runServer(configPath string) error {
	// Acquire process lock
	if err := proc.Acquire(); err != nil {
		return fmt.Errorf("failed to acquire process lock: %w", err)
	}
	defer proc.Release()

	// Load config
	conf, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if err := logx.SetLevel(conf.LogLevel); err != nil {
		return fmt.Errorf("failed to apply log level %q: %w", conf.LogLevel, err)
	}
	logger.Infof("Loaded config with %d servers", len(conf.Servers))

	// Start listening before spawning the accept loop so bind errors fail fast
	gw := gateway.NewGateway(conf)
	if err := gw.Listen(); err != nil {
		return fmt.Errorf("failed to start gateway: %w", err)
	}

	errChan := make(chan error, 1)
	doneChan := make(chan struct{})

	go func() {
		if err := gw.Serve(); err != nil {
			errChan <- err
		}
	}()
	go signalHandler(gw, configPath, doneChan)

	select {
	case err := <-errChan:
		return fmt.Errorf("gateway error: %w", err)
	case <-doneChan:
		logger.Info("Gateway shutdown gracefully.")
		return nil
	}
}

// reloadConfig hot-reloads the config for the running gateway. Called by the
// platform-specific signal handlers.
func reloadConfig(gw *gateway.Gateway, configPath string) {
	newConf, err := config.LoadConfig(configPath)
	if err != nil {
		logger.Errorf("Failed to reload config: %v", err)
		return
	}
	if err := logx.SetLevel(newConf.LogLevel); err != nil {
		logger.Errorf("Failed to apply log level %q: %v", newConf.LogLevel, err)
		return
	}
	gw.UpdateConfig(newConf)
	logger.Infof("Configuration reloaded successfully with %d servers", len(newConf.Servers))
}

func printUsage() {
	out := flag.CommandLine.Output()
	_, _ = fmt.Fprintln(out, "Usage: minecraft-gateway [flags] [command]")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "Commands:")
	_, _ = fmt.Fprintln(out, "  (none)    Start the gateway server")
	_, _ = fmt.Fprintln(out, "  reload    Reload configuration of the running instance")
	_, _ = fmt.Fprintln(out, "  stop      Stop the running instance")
	_, _ = fmt.Fprintln(out, "  help      Show this help")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "Flags:")
	flag.PrintDefaults()
}

func main() {
	defer func() {
		_ = logger.Sync()
	}()

	configPath := flag.String("config", "config.yml", "path to the configuration file")
	flag.Usage = printUsage
	flag.Parse()

	switch flag.Arg(0) {
	case "":
		if err := runServer(*configPath); err != nil {
			logger.Fatalf("%v", err)
		}
	case "reload":
		handleReload()
	case "stop":
		handleStop()
	case "help":
		printUsage()
	default:
		logger.Errorf("Unknown command: %s", flag.Arg(0))
		printUsage()
		os.Exit(1)
	}
}
