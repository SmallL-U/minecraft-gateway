package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"minecraft-gateway/internal/config"
)

type closeWriteTrackingConn struct {
	writeErr         error
	closeWriteCalled bool
}

func (c *closeWriteTrackingConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *closeWriteTrackingConn) Write([]byte) (int, error)        { return 0, c.writeErr }
func (c *closeWriteTrackingConn) Close() error                     { return nil }
func (c *closeWriteTrackingConn) LocalAddr() net.Addr              { return nil }
func (c *closeWriteTrackingConn) RemoteAddr() net.Addr             { return nil }
func (c *closeWriteTrackingConn) SetDeadline(time.Time) error      { return nil }
func (c *closeWriteTrackingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *closeWriteTrackingConn) SetWriteDeadline(time.Time) error { return nil }
func (c *closeWriteTrackingConn) CloseWrite() error {
	c.closeWriteCalled = true
	return nil
}

type acceptErrorListener struct {
	err error
}

func (l *acceptErrorListener) Accept() (net.Conn, error) { return nil, l.err }
func (l *acceptErrorListener) Close() error              { return nil }
func (l *acceptErrorListener) Addr() net.Addr            { return nil }

func TestIsExpectedNetworkError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"use of closed network connection", errors.New("use of closed network connection"), true},
		{"connection reset by peer", errors.New("read tcp 127.0.0.1:1234: connection reset by peer"), true},
		{"broken pipe", errors.New("write tcp 127.0.0.1:1234: broken pipe"), true},
		{"EOF", io.EOF, true},
		{"connection refused", errors.New("dial tcp 127.0.0.1:1234: connect: connection refused"), true},
		{"unrelated error", errors.New("something else went wrong"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isExpectedNetworkError(tt.err); got != tt.want {
				t.Errorf("isExpectedNetworkError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestSendData_Success(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()

	data := []byte("hello backend")

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, len(data))
		_, err := io.ReadFull(server, buf)
		if err != nil {
			received <- nil
			return
		}
		received <- buf
	}()

	if err := sendData(client, data, time.Second); err != nil {
		t.Fatalf("sendData() unexpected error: %v", err)
	}

	got := <-received
	if got == nil {
		t.Fatal("reader side failed to read data")
	}
	if string(got) != string(data) {
		t.Errorf("received %q, want %q", got, data)
	}
}

func TestSendData_Timeout(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()

	// No one reads from server, and net.Pipe is unbuffered, so the write
	// must block until the deadline fires.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Keep the server side open (but not reading) until the test ends.
		<-time.After(200 * time.Millisecond)
	}()

	err := sendData(client, []byte("x"), 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	var netErr net.Error
	if !errors.As(err, &netErr) {
		t.Fatalf("error %v is not a net.Error", err)
	}
	if !netErr.Timeout() {
		t.Errorf("error %v is a net.Error but Timeout() = false", err)
	}

	wg.Wait()
	_ = server.Close()
}

func TestPipeClosesWriteAfterExpectedNetworkError(t *testing.T) {
	dst := &closeWriteTrackingConn{writeErr: errors.New("broken pipe")}

	pipe(dst, strings.NewReader("payload"), "test connection")

	if !dst.closeWriteCalled {
		t.Fatal("pipe() did not call CloseWrite after an expected network error")
	}
}

func TestGatewayUpdateConfigRejectsInvalidReload(t *testing.T) {
	original := &config.Config{ListenAddr: "127.0.0.1:25565"}
	gateway := NewGateway(original)

	if err := gateway.UpdateConfig(nil); err == nil {
		t.Fatal("UpdateConfig(nil) error = nil, want error")
	}
	if gateway.config != original {
		t.Fatal("UpdateConfig(nil) replaced the active config")
	}

	replacement := &config.Config{ListenAddr: "127.0.0.1:25566"}
	if err := gateway.UpdateConfig(replacement); err == nil {
		t.Fatal("UpdateConfig() error = nil for changed listen_addr, want error")
	}
	if gateway.config != original {
		t.Fatal("UpdateConfig() replaced the active config after rejecting listen_addr")
	}

	replacement = &config.Config{ListenAddr: original.ListenAddr, LogLevel: "debug"}
	if err := gateway.UpdateConfig(replacement); err != nil {
		t.Fatalf("UpdateConfig() unexpected error: %v", err)
	}
	if gateway.config != replacement {
		t.Fatal("UpdateConfig() did not replace the active config")
	}
}

func TestGatewayServeRequiresListener(t *testing.T) {
	gateway := NewGateway(&config.Config{})

	if err := gateway.Serve(); err == nil {
		t.Fatal("Serve() error = nil before Listen(), want error")
	}
}

func TestGatewayServeReturnsAcceptError(t *testing.T) {
	acceptErr := errors.New("accept failed")
	gateway := NewGateway(&config.Config{})
	gateway.listener = &acceptErrorListener{err: acceptErr}

	err := gateway.Serve()
	if !errors.Is(err, acceptErr) {
		t.Fatalf("Serve() error = %v, want wrapped %v", err, acceptErr)
	}
	if !strings.Contains(err.Error(), "accept connection") {
		t.Fatalf("Serve() error = %q, want accept context", err)
	}
}

func TestGatewayShutdownForceClosesTrackedConnection(t *testing.T) {
	gateway := NewGateway(&config.Config{})
	tracked, peer := net.Pipe()
	defer func() { _ = tracked.Close() }()
	defer func() { _ = peer.Close() }()

	if !gateway.beginConnection(tracked) {
		t.Fatal("beginConnection() rejected an active gateway")
	}
	readResult := make(chan error, 1)
	go func() {
		buffer := make([]byte, 1)
		_, err := tracked.Read(buffer)
		readResult <- err
		gateway.endConnection(tracked)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := gateway.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error: %v", err)
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("shutdown context error = %v, want deadline exceeded", ctx.Err())
	}

	select {
	case err := <-readResult:
		if err == nil {
			t.Fatal("tracked connection read returned nil error after forced shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("tracked connection was not closed by forced shutdown")
	}
	if _, err := tracked.Write([]byte("x")); err == nil {
		t.Fatal("tracked connection remained writable after forced shutdown")
	}

	gateway.lifecycleMutex.Lock()
	trackedConnections := len(gateway.connections)
	gateway.lifecycleMutex.Unlock()
	if trackedConnections != 0 {
		t.Fatalf("tracked connections after Shutdown() = %d, want 0", trackedConnections)
	}

	waitDone := make(chan struct{})
	go func() {
		gateway.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("Wait() did not return after forced shutdown")
	}
}

func TestGatewayLoopbackLifecycleAndRouting(t *testing.T) {
	backend, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for backend: %v", err)
	}
	defer func() { _ = backend.Close() }()

	defaultBackend, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for default backend: %v", err)
	}
	defer func() { _ = defaultBackend.Close() }()

	if tcpListener, ok := backend.(*net.TCPListener); ok {
		if err := tcpListener.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatalf("set backend accept deadline: %v", err)
		}
	}

	const hostname = "play.example.com"
	configPath := filepath.Join(t.TempDir(), "config.yml")
	configData := fmt.Sprintf(`
timeout: 2s
listen_addr: "127.0.0.1:0"
default: %q
log_level: error
whitelist:
  - "127.0.0.1/32"
servers:
  - name: %q
    address: %q
`, defaultBackend.Addr().String(), hostname, backend.Addr().String())
	if err := os.WriteFile(configPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	conf, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	gateway := NewGateway(conf)
	if err := gateway.Listen(); err != nil {
		t.Fatalf("Listen() error: %v", err)
	}
	defer func() { _ = gateway.Stop() }()

	serveResult := make(chan error, 1)
	go func() {
		serveResult <- gateway.Serve()
	}()

	handshake := buildTestHandshake(hostname)
	clientPayload := []byte("client payload")
	wantBackendData := append(append([]byte(nil), handshake...), clientPayload...)
	backendResponse := []byte("backend response")
	type backendResult struct {
		data []byte
		err  error
	}
	backendResultCh := make(chan backendResult, 1)
	go func() {
		conn, err := backend.Accept()
		if err != nil {
			backendResultCh <- backendResult{err: err}
			return
		}
		defer func() { _ = conn.Close() }()
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			backendResultCh <- backendResult{err: err}
			return
		}

		data, err := io.ReadAll(conn)
		if err != nil {
			backendResultCh <- backendResult{err: err}
			return
		}
		if _, err := conn.Write(backendResponse); err != nil {
			backendResultCh <- backendResult{err: err}
			return
		}
		backendResultCh <- backendResult{data: data}
	}()

	client, err := net.DialTimeout("tcp", gateway.listener.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial gateway: %v", err)
	}
	defer func() { _ = client.Close() }()
	if err := client.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set client deadline: %v", err)
	}
	if _, err := io.Copy(client, bytes.NewReader(wantBackendData)); err != nil {
		t.Fatalf("write client data: %v", err)
	}
	tcpClient, ok := client.(*net.TCPConn)
	if !ok {
		t.Fatalf("client connection type = %T, want *net.TCPConn", client)
	}
	if err := tcpClient.CloseWrite(); err != nil {
		t.Fatalf("close client write side: %v", err)
	}

	response, err := io.ReadAll(client)
	if err != nil {
		t.Fatalf("read backend response: %v", err)
	}
	if !bytes.Equal(response, backendResponse) {
		t.Fatalf("gateway response = %q, want %q", response, backendResponse)
	}

	select {
	case result := <-backendResultCh:
		if result.err != nil {
			t.Fatalf("backend error: %v", result.err)
		}
		if !bytes.Equal(result.data, wantBackendData) {
			t.Fatalf("backend data = %x, want %x", result.data, wantBackendData)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for backend")
	}

	if err := gateway.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("Serve() error after Stop(): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Serve() to return")
	}

	waitDone := make(chan struct{})
	go func() {
		gateway.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for active connections")
	}
}

func buildTestHandshake(address string) []byte {
	payload := []byte{0x00, 0x2f, byte(len(address))}
	payload = append(payload, address...)
	payload = append(payload, 0x63, 0xdd, 0x01)
	return append([]byte{byte(len(payload))}, payload...)
}
