package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"minecraft-gateway/internal/config"
	"minecraft-gateway/internal/gateway"
	"minecraft-gateway/internal/logx"
	"minecraft-gateway/internal/proc"
)

var logger = logx.GetLogger()

const shutdownTimeout = 5 * time.Second

func handleReload() error {
	if err := proc.SendReload(); err != nil {
		return fmt.Errorf("failed to send reload signal: %w", err)
	}
	logger.Info("Reload signal sent successfully")
	return nil
}

func handleStop() error {
	if err := proc.SendStop(); err != nil {
		return fmt.Errorf("failed to send stop signal: %w", err)
	}
	logger.Info("Stop signal sent successfully")
	return nil
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

	serveChan := make(chan error, 1)
	signalChan := make(chan error, 1)

	go func() {
		serveChan <- gw.Serve()
	}()
	go func() {
		signalChan <- signalHandler(gw, configPath)
	}()

	var serveErr error
	serveFinished := false
	var signalErr error
	select {
	case serveErr = <-serveChan:
		serveFinished = true
	case signalErr = <-signalChan:
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := gw.Shutdown(ctx)
	if !serveFinished {
		serveErr = <-serveChan
	}

	if signalErr != nil {
		return fmt.Errorf("signal handler error: %w", signalErr)
	}
	if shutdownErr != nil {
		return fmt.Errorf("gateway shutdown error: %w", shutdownErr)
	}
	if serveErr != nil {
		return fmt.Errorf("gateway error: %w", serveErr)
	}
	logger.Info("Gateway shutdown complete")
	return nil
}

// reloadConfig hot-reloads the config for the running gateway. Called by the
// platform-specific signal handlers.
func reloadConfig(gw *gateway.Gateway, configPath string) {
	newConf, err := config.LoadConfig(configPath)
	if err != nil {
		logger.Errorf("Failed to reload config: %v", err)
		return
	}
	if err := gw.UpdateConfig(newConf); err != nil {
		logger.Errorf("Failed to apply reloaded config: %v", err)
		return
	}
	if err := logx.SetLevel(newConf.LogLevel); err != nil {
		logger.Errorf("Failed to apply log level %q: %v", newConf.LogLevel, err)
		return
	}
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

	var err error
	switch flag.Arg(0) {
	case "":
		err = runServer(*configPath)
	case "reload":
		err = handleReload()
	case "stop":
		err = handleStop()
	case "help":
		printUsage()
	default:
		printUsage()
		err = fmt.Errorf("unknown command: %s", flag.Arg(0))
	}

	if err == nil {
		return
	}
	logger.Errorf("%v", err)
	_ = logger.Sync()
	os.Exit(1)
}
