package gateway

import (
	"errors"
	"fmt"
	"minecraft-gateway/config"
	"minecraft-gateway/logx"
	"minecraft-gateway/protocol"
	"net"
	"strings"
	"sync"
	"time"
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

func (g *Gateway) selectBackend(serverAddr string) string {
	// TODO: Implement backend selection logic based on serverAddr
	return ""
}

func (g *Gateway) handleConnection(clientConn net.Conn) {
	defer func() {
		_ = clientConn.Close()
	}()
	conf := g.Config
	originalClientAddr := clientConn.RemoteAddr()

	// parse proxy protocol if enabled
	if conf.ProxyProtocol.ReceiveFromDownstream {
		header, err := protocol.ParseProxyProtocol(clientConn)
		if err != nil {
			logger.Errorf("Failed to parse proxy protocol header from %s: %s", originalClientAddr, err)
			return
		}
		logger.Debugf("Received proxy protocol header from %s: %+v", originalClientAddr, header)
	}

	// parse handshake
	handshake, data, err := protocol.ParseHandshake(clientConn)
	if err != nil {
		logger.Errorf("Failed to parse handshake from %s: %s", originalClientAddr, err)
		return
	}
	logger.Debugf("Received handshake from %s: %+v", originalClientAddr, handshake)

	// select backend
	backendAddr := g.selectBackend(handshake.ServerAddress)
	if backendAddr == "" {
		logger.Warnf("No backend selected for server address %s", handshake.ServerAddress)
		return
	}

	// dial backend
	logger.Infof("Routing connection from %s to backend %s", originalClientAddr, backendAddr)
	backendConn, err := net.DialTimeout("tcp", backendAddr, conf.Timeout*time.Second)
	if err != nil {
		logger.Errorf("Failed to connect to backend %s: %s", backendAddr, err)
		return
	}
	defer func() {
		_ = backendConn.Close()
	}()
	// send proxy protocol header if enabled
	if conf.ProxyProtocol.SendToUpstream {
		// TODO: Implement sending proxy protocol header to upstream
	}

	// TODO: Forward handshake data to backend and start bidirectional forwarding
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
