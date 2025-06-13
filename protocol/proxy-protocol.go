package protocol

import (
	"fmt"
	"net"
	"strings"
)

type ProxyProtocolHeader struct {
	Protocol string
	SrcIP    string
	DestIP   string
	SrcPort  uint16
	DestPort uint16
}

func BuildProxyProtocolHeader(protocol string, srcIP string, destIP string, srcPort uint16, destPort uint16) *ProxyProtocolHeader {
	return &ProxyProtocolHeader{
		Protocol: protocol,
		SrcIP:    srcIP,
		DestIP:   destIP,
		SrcPort:  srcPort,
		DestPort: destPort,
	}
}

func ParseProxyProtocol(conn net.Conn) (*ProxyProtocolHeader, error) {
	// TODO: Implement the logic to read the proxy protocol header from the connection.
	return nil, nil
}

func (h *ProxyProtocolHeader) ToBytes() ([]byte, error) {
	var builder strings.Builder
	_, err := fmt.Fprintf(&builder, "PROXY %s %s %s %d %d\r\n", h.Protocol, h.SrcIP, h.DestIP, h.SrcPort, h.DestPort)
	if err != nil {
		return nil, err
	}
	return []byte(builder.String()), nil
}
