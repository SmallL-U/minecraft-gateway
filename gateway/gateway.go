package gateway

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"minecraft-gateway/config"
	"minecraft-gateway/logx"
	"minecraft-gateway/protocol"
	"minecraft-gateway/whitelist"
	"net"
	"strings"
	"sync"
	"time"
)

var logger = logx.GetLogger()

type Gateway struct {
	config         *config.Config
	configMutex    sync.RWMutex
	whitelist      *whitelist.Whitelist
	whitelistMutex sync.RWMutex
	listener       net.Listener
}

func NewGateway(conf *config.Config, allowlist *whitelist.Whitelist) *Gateway {
	return &Gateway{config: conf, whitelist: allowlist}
}

func (g *Gateway) UpdateWhitelist(allowlist *whitelist.Whitelist) {
	g.whitelistMutex.Lock()
	defer g.whitelistMutex.Unlock()
	g.whitelist = allowlist
}

func (g *Gateway) UpdateConfig(conf *config.Config) {
	g.configMutex.Lock()
	defer g.configMutex.Unlock()
	g.config = conf
}

func (g *Gateway) selectBackend(serverAddr string) string {
	if backend, ok := g.config.Backends[serverAddr]; ok {
		return backend
	}
	return g.config.Default
}

func tcpAddrFromIPPort(ipStr string, port uint16) (*net.TCPAddr, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP: %s", ipStr)
	}
	// 对 IPv6，要用方括号包裹
	var addrStr string
	if ip.To4() == nil { // 不是IPv4就是IPv6
		addrStr = fmt.Sprintf("[%s]:%d", ipStr, port)
	} else {
		addrStr = fmt.Sprintf("%s:%d", ipStr, port)
	}
	return net.ResolveTCPAddr("tcp", addrStr)
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
	if err != nil {
		return err
	}
	return nil
}

func isExpectedNetworkError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// 常见的预期网络错误
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

	// parse proxy protocol if enabled
	if conf.ProxyProtocol.ReceiveFromDownstream {
		header, err := protocol.ParseProxyProtocol(reader)
		if err != nil {
			logger.Errorf("Failed to parse proxy protocol header from %s: %s", clientAddr, err)
			return
		}
		tcpAddr, err := tcpAddrFromIPPort(header.SrcIP, header.SrcPort)
		if err != nil {
			logger.Errorf("Failed to resolve TCP address from proxy protocol header: %s", err)
			return
		}
		clientAddr = tcpAddr
		logger.Debugf("Received proxy protocol header from %s: %+v", clientAddr, header)
	}

	// parse handshake
	handshake, data, err := protocol.ParseHandshake(reader)
	if err != nil {
		logger.Errorf("Failed to parse handshake from %s: %s", clientAddr, err)
		return
	}
	logger.Debugf("Received handshake from %s: %+v", clientAddr, handshake)

	// select backend
	backendAddr := g.selectBackend(handshake.ServerAddress)
	if backendAddr == "" {
		logger.Warnf("No backend selected for server address %s", handshake.ServerAddress)
		return
	}

	// dial backend
	logger.Infof("Routing connection from %s to backend %s", clientAddr, backendAddr)
	backendConn, err := net.DialTimeout("tcp", backendAddr, conf.Timeout)
	if err != nil {
		logger.Errorf("Failed to connect to backend %s: %s", backendAddr, err)
		return
	}
	defer func() {
		_ = backendConn.Close()
	}()
	// send proxy protocol header if enabled
	if conf.ProxyProtocol.SendToUpstream {
		// safely extract client address
		clientTCPAddr, ok := clientAddr.(*net.TCPAddr)
		if !ok {
			logger.Errorf("Expected TCP address for client, got %T", clientAddr)
			return
		}
		// safely extract backend address
		backendTCPAddr, ok := backendConn.RemoteAddr().(*net.TCPAddr)
		if !ok {
			logger.Errorf("Expected TCP address for backend, got %T", backendConn.RemoteAddr())
			return
		}
		// determine protocol type based on IP version
		protocolType := "TCP4"
		if clientTCPAddr.IP.To4() == nil {
			protocolType = "TCP6"
		}
		header := protocol.BuildProxyProtocolHeader(
			protocolType,
			clientTCPAddr.IP.String(),
			backendTCPAddr.IP.String(),
			uint16(clientTCPAddr.Port),
			uint16(backendTCPAddr.Port),
		)
		bytes, err := header.ToBytes()
		if err != nil {
			logger.Errorf("Failed to serialize header: %s", err)
			return
		}
		if err := sendData(backendConn, bytes); err != nil {
			logger.Errorf("Failed to send proxy protocol header to backend %s: %s", backendAddr, err)
			return
		}
	}

	// resend handshake data to backend
	if err := sendData(backendConn, data); err != nil {
		logger.Errorf("Failed to send handshake data to backend %s: %s", backendAddr, err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	// forward client to backend
	go func() {
		defer wg.Done()
		if _, err := io.Copy(backendConn, reader); err != nil {
			if isExpectedNetworkError(err) {
				return
			}
			logger.Errorf("Error forwarding data from client %s to backend %s: %s", clientAddr, backendAddr, err)
		}
		// close write side to signal end of data
		if closer, ok := backendConn.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
	}()
	// forward backend to client
	go func() {
		defer wg.Done()
		if _, err := io.Copy(clientConn, backendConn); err != nil {
			if isExpectedNetworkError(err) {
				return
			}
			logger.Errorf("Error forwarding data from backend %s to client %s: %s", backendAddr, clientAddr, err)
		}
		// close write side to signal end of data
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
		// whitelist
		tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
		if !ok {
			logger.Warnf("Received connection from non-TCP address: %s", conn.RemoteAddr())
			_ = conn.Close()
			continue
		}
		g.whitelistMutex.RLock()
		allowed := g.whitelist.Allowed(tcpAddr.IP)
		g.whitelistMutex.RUnlock()
		if !allowed {
			logger.Debugf("Connection from %s is not allowed by whitelist", tcpAddr.IP)
			_ = conn.Close()
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
