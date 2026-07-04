package gateway

import (
	"bufio"
	"errors"
	"fmt"
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

func sendData(dst net.Conn, data []byte, timeout time.Duration) error {
	err := dst.SetWriteDeadline(time.Now().Add(timeout))
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
	if idx := strings.IndexByte(serverName, '\x00'); idx != -1 {
		serverName = serverName[:idx]
	}

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
		if err := sendData(backendConn, headerBytes, conf.Timeout); err != nil {
			logger.Errorf("Failed to send proxy protocol header to backend %s: %s", backendAddr, err)
			return
		}
	}

	// Resend handshake data to backend
	if err := sendData(backendConn, data, conf.Timeout); err != nil {
		logger.Errorf("Failed to send handshake data to backend %s: %s", backendAddr, err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Forward client to backend
	go func() {
		defer wg.Done()
		pipe(backendConn, reader, fmt.Sprintf("client %s to backend %s", clientAddr, backendAddr))
	}()

	// Forward backend to client
	go func() {
		defer wg.Done()
		pipe(clientConn, backendConn, fmt.Sprintf("backend %s to client %s", backendAddr, clientAddr))
	}()

	wg.Wait()
	logger.Infof("Connection closed for %s", clientAddr)
}

// pipe copies src to dst until EOF, then half-closes dst so the peer sees EOF.
func pipe(dst net.Conn, src io.Reader, desc string) {
	if _, err := io.Copy(dst, src); err != nil {
		if isExpectedNetworkError(err) {
			return
		}
		logger.Errorf("Error forwarding data from %s: %s", desc, err)
	}
	if closer, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = closer.CloseWrite()
	}
}

// Listen opens the listening socket. It must be called before Serve.
func (g *Gateway) Listen() error {
	listener, err := net.Listen("tcp", g.config.ListenAddr)
	if err != nil {
		return err
	}
	g.listener = listener
	logger.Infof("Gateway listening on %s", g.config.ListenAddr)
	return nil
}

// Serve accepts connections until the listener is closed via Stop.
func (g *Gateway) Serve() error {
	for {
		conn, err := g.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				logger.Info("Listener closed, shutting down gracefully")
				return nil
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
