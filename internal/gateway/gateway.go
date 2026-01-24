package gateway

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"minecraft-gateway/internal/config"
	"minecraft-gateway/internal/logx"
	"minecraft-gateway/internal/protocol"
)

var logger = logx.GetLogger()

type Gateway struct {
	config      *config.Config
	configMutex sync.RWMutex
	listener    net.Listener
}

func NewGateway(conf *config.Config) *Gateway {
	return &Gateway{config: conf}
}

func (g *Gateway) UpdateConfig(conf *config.Config) {
	g.configMutex.Lock()
	defer g.configMutex.Unlock()
	g.config = conf
}

func sendData(dst net.Conn, data []byte) error {
	err := dst.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		return err
	}
	defer func() {
		if err := dst.SetWriteDeadline(time.Time{}); err != nil {
			logger.Warnf("Failed to reset write deadline: %s", err)
		}
	}()
	_, err = dst.Write(data)
	return err
}

func isExpectedNetworkError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	expectedErrors := []string{
		"use of closed network connection",
		"connection reset by peer",
		"broken pipe",
		"EOF",
		"connection refused",
		"network is unreachable",
		"no route to host",
	}

	for _, expected := range expectedErrors {
		if strings.Contains(errStr, expected) {
			return true
		}
	}

	return false
}

func (g *Gateway) handleConnection(clientConn net.Conn) {
	defer func() {
		_ = clientConn.Close()
	}()

	g.configMutex.RLock()
	conf := g.config
	g.configMutex.RUnlock()

	clientAddr := clientConn.RemoteAddr()
	reader := bufio.NewReader(clientConn)

	// Check global whitelist first (before parsing anything)
	tcpAddr, ok := clientAddr.(*net.TCPAddr)
	if !ok {
		logger.Warnf("Connection from non-TCP address: %s", clientAddr)
		return
	}
	if !conf.IsAllowedByGlobal(tcpAddr.IP) {
		logger.Debugf("Connection from %s is not allowed by global whitelist", tcpAddr.IP)
		return
	}

	// Parse proxy protocol if enabled globally
	if conf.ProxyProtocol.ReceiveFromDownstream {
		header, err := protocol.ParseProxyProtocol(reader)
		if err != nil {
			logger.Errorf("Failed to parse proxy protocol header from %s: %s", clientAddr, err)
			return
		}
		clientAddr = header.SrcAddr
		logger.Debugf("Received proxy protocol header from %s", clientAddr)
	}

	// Parse handshake
	handshake, data, err := protocol.ParseHandshake(reader)
	if err != nil {
		logger.Errorf("Failed to parse handshake from %s: %s", clientAddr, err)
		return
	}
	logger.Debugf("Received handshake from %s: %+v", clientAddr, handshake)

	serverName := handshake.ServerAddress

	// Check server-specific whitelist
	if clientTCP, ok := clientAddr.(*net.TCPAddr); ok {
		if !conf.IsAllowed(serverName, clientTCP.IP) {
			logger.Debugf("Connection from %s is not allowed by whitelist for server %s", clientTCP.IP, serverName)
			return
		}
	}

	// Get server address
	backendAddr := conf.GetServerAddress(serverName)
	if backendAddr == "" {
		logger.Warnf("No backend selected for server address %s", serverName)
		return
	}

	// Get proxy protocol config for this server
	proxyProtocol := conf.GetProxyProtocol(serverName)

	// Dial backend
	logger.Infof("Routing connection from %s to backend %s", clientAddr, backendAddr)
	backendConn, err := net.DialTimeout("tcp", backendAddr, conf.Timeout)
	if err != nil {
		logger.Errorf("Failed to connect to backend %s: %s", backendAddr, err)
		return
	}
	defer func() {
		_ = backendConn.Close()
	}()

	// Send proxy protocol header if enabled for this server
	if proxyProtocol.SendToUpstream {
		headerBytes, err := protocol.BuildProxyProtocolV1Header(clientAddr, backendConn.RemoteAddr())
		if err != nil {
			logger.Errorf("Failed to build proxy protocol header: %s", err)
			return
		}
		if err := sendData(backendConn, headerBytes); err != nil {
			logger.Errorf("Failed to send proxy protocol header to backend %s: %s", backendAddr, err)
			return
		}
	}

	// Resend handshake data to backend
	if err := sendData(backendConn, data); err != nil {
		logger.Errorf("Failed to send handshake data to backend %s: %s", backendAddr, err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Forward client to backend
	go func() {
		defer wg.Done()
		if _, err := io.Copy(backendConn, reader); err != nil {
			if isExpectedNetworkError(err) {
				return
			}
			logger.Errorf("Error forwarding data from client %s to backend %s: %s", clientAddr, backendAddr, err)
		}
		if closer, ok := backendConn.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
	}()

	// Forward backend to client
	go func() {
		defer wg.Done()
		if _, err := io.Copy(clientConn, backendConn); err != nil {
			if isExpectedNetworkError(err) {
				return
			}
			logger.Errorf("Error forwarding data from backend %s to client %s: %s", backendAddr, clientAddr, err)
		}
		if closer, ok := clientConn.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
	}()

	wg.Wait()
	logger.Infof("Connection closed for %s", clientAddr)
}

func (g *Gateway) Start() error {
	logger.Info("Starting gateway...")
	listener, err := net.Listen("tcp", g.config.ListenAddr)
	if err != nil {
		return err
	}
	g.listener = listener
	logger.Infof("Gateway listening on %s", g.config.ListenAddr)

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
