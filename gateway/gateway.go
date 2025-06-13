package gateway

import (
	"errors"
	"fmt"
	"minecraft-gateway/config"
	"minecraft-gateway/logx"
	"net"
	"strings"
	"sync"
)

var logger = logx.GetLogger()

type Gateway struct {
	Config      *config.Config
	configMutex sync.RWMutex
	listener    net.Listener
}

func NewGateway(cfg *config.Config) *Gateway {
	return &Gateway{Config: cfg}
}

func (g *Gateway) handleConnection(clientConn net.Conn) {
	// TODO: Implement connection handling logic
}

func (g *Gateway) Start() error {
	logger.Info("Starting gateway...")
	listener, err := net.Listen("tcp", g.Config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to start gateway: %s", err)
	}
	g.listener = listener
	logger.Infof("Gateway listening on %s", g.Config.ListenAddr)
	for {
		conn, err := listener.Accept()
		if err != nil {
			var opErr *net.OpError
			if errors.As(err, &opErr) && opErr.Op == "accept" {
				if strings.Contains(err.Error(), "use of closed network connection") {
					logger.Info("Listener closed, shutting down gracefully")
					return nil
				}
			}
			logger.Errorf("Failed to accept connection: %s", err)
			continue
		}
		go g.handleConnection(conn)
	}
}

func (g *Gateway) Stop() error {
	if g.listener != nil {
		return g.listener.Close()
	}
	return nil
}
