package gateway

import (
	"errors"
	"fmt"
	"io"
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
	conf := g.Config
	clientAddr := clientConn.RemoteAddr()

	// parse proxy protocol if enabled
	if conf.ProxyProtocol.ReceiveFromDownstream {
		header, err := protocol.ParseProxyProtocol(clientConn)
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
	handshake, data, err := protocol.ParseHandshake(clientConn)
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
		header := protocol.BuildProxyProtocolHeader(
			"TCP4",
			clientAddr.(*net.TCPAddr).IP.String(),
			backendConn.RemoteAddr().(*net.TCPAddr).IP.String(),
			uint16(clientAddr.(*net.TCPAddr).Port),
			uint16(backendConn.RemoteAddr().(*net.TCPAddr).Port),
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

	// send handshake data to backend
	if err := sendData(backendConn, data); err != nil {
		logger.Errorf("Failed to send handshake data to backend %s: %s", backendAddr, err)
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	// forward client to backend
	go func() {
		defer wg.Done()
		defer func() {
			_ = backendConn.Close()
		}()
		if _, err := io.Copy(backendConn, clientConn); err != nil {
			if isExpectedNetworkError(err) {
				return
			}
			logger.Errorf("Error forwarding data from client %s to backend %s: %s", clientAddr, backendAddr, err)
		}
	}()
	// forward backend to client
	go func() {
		defer wg.Done()
		defer func() {
			_ = clientConn.Close()
		}()
		if _, err := io.Copy(clientConn, backendConn); err != nil {
			if !isExpectedNetworkError(err) {
				return
			}
			logger.Errorf("Error forwarding data from backend %s to client %s: %s", backendAddr, clientAddr, err)
		}
	}()

	wg.Wait()
	logger.Infof("Connection closed for %s", clientAddr)
}

func (g *Gateway) Start() error {
	logger.Info("Starting gateway...")
	listener, err := net.Listen("tcp", g.Config.ListenAddr)
	if err != nil {
		return err
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
