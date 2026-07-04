package protocol

import (
	"bufio"
	"fmt"
	"net"

	proxyproto "github.com/pires/go-proxyproto"
)

// ProxyProtocolHeader represents parsed proxy protocol header info.
type ProxyProtocolHeader struct {
	SrcAddr net.Addr
	DstAddr net.Addr
}

// ParseProxyProtocol parses PROXY protocol header (supports both v1 and v2).
func ParseProxyProtocol(reader *bufio.Reader) (*ProxyProtocolHeader, error) {
	header, err := proxyproto.Read(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy protocol: %w", err)
	}

	return &ProxyProtocolHeader{
		SrcAddr: header.SourceAddr,
		DstAddr: header.DestinationAddr,
	}, nil
}

// BuildProxyProtocolV1Header builds a PROXY protocol v1 header for sending to upstream.
func BuildProxyProtocolV1Header(srcAddr, dstAddr net.Addr) ([]byte, error) {
	srcTCP, ok := srcAddr.(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("source address is not TCP: %T", srcAddr)
	}
	dstTCP, ok := dstAddr.(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("destination address is not TCP: %T", dstAddr)
	}

	transportProto := proxyproto.TCPv4
	if srcTCP.IP.To4() == nil {
		transportProto = proxyproto.TCPv6
	}

	header := &proxyproto.Header{
		Version:           1,
		Command:           proxyproto.PROXY,
		TransportProtocol: transportProto,
		SourceAddr:        srcTCP,
		DestinationAddr:   dstTCP,
	}

	return header.Format()
}
