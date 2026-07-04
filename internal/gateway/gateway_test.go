package gateway

import (
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

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
	if errors.As(err, &netErr) {
		if !netErr.Timeout() {
			t.Errorf("error %v is a net.Error but Timeout() = false", err)
		}
	}

	wg.Wait()
	_ = server.Close()
}
