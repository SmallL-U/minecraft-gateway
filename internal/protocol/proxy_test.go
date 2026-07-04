package protocol

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

func TestBuildProxyProtocolV1Header(t *testing.T) {
	t.Run("IPv4 addresses produce a TCP4 header", func(t *testing.T) {
		src := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 1000}
		dst := &net.TCPAddr{IP: net.ParseIP("5.6.7.8"), Port: 2000}

		header, err := BuildProxyProtocolV1Header(src, dst)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		s := string(header)
		if !strings.HasPrefix(s, "PROXY TCP4 ") {
			t.Errorf("header = %q, want prefix %q", s, "PROXY TCP4 ")
		}
		if !strings.HasSuffix(s, "\r\n") {
			t.Errorf("header = %q, want suffix %q", s, "\\r\\n")
		}
	})

	t.Run("IPv6 addresses produce a TCP6 header", func(t *testing.T) {
		src := &net.TCPAddr{IP: net.ParseIP("2001:db8::1"), Port: 1000}
		dst := &net.TCPAddr{IP: net.ParseIP("2001:db8::2"), Port: 2000}

		header, err := BuildProxyProtocolV1Header(src, dst)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		s := string(header)
		if !strings.HasPrefix(s, "PROXY TCP6 ") {
			t.Errorf("header = %q, want prefix %q", s, "PROXY TCP6 ")
		}
		if !strings.HasSuffix(s, "\r\n") {
			t.Errorf("header = %q, want suffix %q", s, "\\r\\n")
		}
	})

	t.Run("non-TCP source address errors", func(t *testing.T) {
		src := &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 1000}
		dst := &net.TCPAddr{IP: net.ParseIP("5.6.7.8"), Port: 2000}

		_, err := BuildProxyProtocolV1Header(src, dst)
		if err == nil {
			t.Fatal("expected error for non-TCP source address, got nil")
		}
	})

	t.Run("non-TCP destination address errors", func(t *testing.T) {
		src := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 1000}
		dst := &net.UDPAddr{IP: net.ParseIP("5.6.7.8"), Port: 2000}

		_, err := BuildProxyProtocolV1Header(src, dst)
		if err == nil {
			t.Fatal("expected error for non-TCP destination address, got nil")
		}
	})
}

func TestParseProxyProtocol_Valid(t *testing.T) {
	payload := "some trailing payload bytes"
	input := "PROXY TCP4 1.2.3.4 5.6.7.8 1000 2000\r\n" + payload

	reader := bufio.NewReader(strings.NewReader(input))
	header, err := ParseProxyProtocol(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	srcTCP, ok := header.SrcAddr.(*net.TCPAddr)
	if !ok {
		t.Fatalf("SrcAddr type = %T, want *net.TCPAddr", header.SrcAddr)
	}
	if srcTCP.IP.String() != "1.2.3.4" {
		t.Errorf("SrcAddr.IP = %q, want %q", srcTCP.IP.String(), "1.2.3.4")
	}
	if srcTCP.Port != 1000 {
		t.Errorf("SrcAddr.Port = %d, want %d", srcTCP.Port, 1000)
	}

	dstTCP, ok := header.DstAddr.(*net.TCPAddr)
	if !ok {
		t.Fatalf("DstAddr type = %T, want *net.TCPAddr", header.DstAddr)
	}
	if dstTCP.IP.String() != "5.6.7.8" {
		t.Errorf("DstAddr.IP = %q, want %q", dstTCP.IP.String(), "5.6.7.8")
	}
	if dstTCP.Port != 2000 {
		t.Errorf("DstAddr.Port = %d, want %d", dstTCP.Port, 2000)
	}
}

func TestParseProxyProtocol_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"not a proxy protocol header at all", "this is not a proxy protocol header"},
		{"proxy signature but malformed body", "PROXY BOGUS\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			_, err := ParseProxyProtocol(reader)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
