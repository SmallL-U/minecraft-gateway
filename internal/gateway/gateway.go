package gateway

import (
	"bufio"
	"context"
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

	listener        net.Listener
	lifecycleMutex  sync.Mutex
	stopping        bool
	forcing         bool
	connections     map[net.Conn]struct{}
	connectionGroup sync.WaitGroup
	dialContext     context.Context
	cancelDials     context.CancelFunc
}

func NewGateway(conf *config.Config) *Gateway {
	dialContext, cancelDials := context.WithCancel(context.Background())
	return &Gateway{
		config:      conf,
		connections: make(map[net.Conn]struct{}),
		dialContext: dialContext,
		cancelDials: cancelDials,
	}
}

func (g *Gateway) UpdateConfig(conf *config.Config) error {
	if conf == nil {
		return errors.New("config cannot be nil")
	}

	g.configMutex.Lock()
	defer g.configMutex.Unlock()
	if g.config != nil && conf.ListenAddr != g.config.ListenAddr {
		return fmt.Errorf(
			"listen_addr cannot be changed during reload: %q to %q",
			g.config.ListenAddr,
			conf.ListenAddr,
		)
	}
	g.config = conf
	return nil
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

func (g *Gateway) beginConnection(conn net.Conn) bool {
	g.lifecycleMutex.Lock()
	defer g.lifecycleMutex.Unlock()
	if g.stopping {
		return false
	}

	g.connections[conn] = struct{}{}
	g.connectionGroup.Add(1)
	return true
}

func (g *Gateway) trackConnection(conn net.Conn) bool {
	g.lifecycleMutex.Lock()
	defer g.lifecycleMutex.Unlock()
	if g.forcing {
		return false
	}

	g.connections[conn] = struct{}{}
	return true
}

func (g *Gateway) untrackConnection(conn net.Conn) {
	g.lifecycleMutex.Lock()
	delete(g.connections, conn)
	g.lifecycleMutex.Unlock()
}

func (g *Gateway) endConnection(conn net.Conn) {
	g.untrackConnection(conn)
	g.connectionGroup.Done()
}

func (g *Gateway) handleConnection(clientConn net.Conn) {
	defer func() {
		_ = clientConn.Close()
		g.endConnection(clientConn)
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
	dialer := net.Dialer{Timeout: conf.Timeout}
	backendConn, err := dialer.DialContext(g.dialContext, "tcp", backendAddr)
	if err != nil {
		logger.Errorf("Failed to connect to backend %s: %s", backendAddr, err)
		return
	}
	if !g.trackConnection(backendConn) {
		_ = backendConn.Close()
		return
	}
	defer func() {
		_ = backendConn.Close()
		g.untrackConnection(backendConn)
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
	defer func() {
		if closer, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
	}()

	_, err := io.Copy(dst, src)
	if err == nil || isExpectedNetworkError(err) {
		return
	}
	logger.Errorf("Error forwarding data from %s: %s", desc, err)
}

// Listen opens the listening socket. It must be called before Serve.
func (g *Gateway) Listen() error {
	g.configMutex.RLock()
	conf := g.config
	g.configMutex.RUnlock()
	if conf == nil {
		return errors.New("config cannot be nil")
	}

	listener, err := net.Listen("tcp", conf.ListenAddr)
	if err != nil {
		return err
	}

	g.lifecycleMutex.Lock()
	defer g.lifecycleMutex.Unlock()
	if g.stopping {
		_ = listener.Close()
		return errors.New("gateway is stopping")
	}
	if g.listener != nil {
		_ = listener.Close()
		return errors.New("gateway is already listening")
	}
	g.listener = listener
	logger.Infof("Gateway listening on %s", conf.ListenAddr)
	return nil
}

// Serve accepts connections until the listener is closed via Stop.
func (g *Gateway) Serve() error {
	g.lifecycleMutex.Lock()
	listener := g.listener
	g.lifecycleMutex.Unlock()
	if listener == nil {
		return errors.New("gateway listener is not initialized")
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				logger.Info("Listener closed")
				return nil
			}
			return fmt.Errorf("accept connection: %w", err)
		}

		if !g.beginConnection(conn) {
			_ = conn.Close()
			return nil
		}
		go func(conn net.Conn) {
			g.handleConnection(conn)
		}(conn)
	}
}

// Wait waits for all accepted connections to finish. Serve must return before
// Wait is called so no new connections can be added to the wait group.
func (g *Gateway) Wait() {
	g.connectionGroup.Wait()
}

func (g *Gateway) Stop() error {
	g.lifecycleMutex.Lock()
	g.stopping = true
	listener := g.listener
	g.lifecycleMutex.Unlock()
	if listener == nil {
		return nil
	}

	err := listener.Close()
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (g *Gateway) forceCloseConnections() {
	g.lifecycleMutex.Lock()
	g.forcing = true
	connections := make([]net.Conn, 0, len(g.connections))
	for conn := range g.connections {
		connections = append(connections, conn)
	}
	g.lifecycleMutex.Unlock()

	g.cancelDials()
	for _, conn := range connections {
		_ = conn.Close()
	}
}

// Shutdown stops accepting new connections, waits for active connections to
// finish, and force-closes them if the context expires.
func (g *Gateway) Shutdown(ctx context.Context) error {
	listenerErr := g.Stop()

	done := make(chan struct{})
	go func() {
		g.Wait()
		close(done)
	}()

	select {
	case <-done:
		g.cancelDials()
		return listenerErr
	case <-ctx.Done():
		logger.Warnf("Graceful shutdown deadline reached: %v; closing active connections", ctx.Err())
		g.forceCloseConnections()
		<-done
		return listenerErr
	}
}
