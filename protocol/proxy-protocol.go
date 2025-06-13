package protocol

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
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
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	parts := strings.Split(line, " ")
	if len(parts) < 2 || parts[0] != "PROXY" {
		return nil, fmt.Errorf("invalid PROXY header: %s", line)
	}
	proto := parts[1]
	if len(parts) != 6 {
		return nil, fmt.Errorf("invalid PROXY header format: %s", line)
	}
	srcIP := parts[2]
	dstIP := parts[3]
	srcPort64, err := strconv.ParseUint(parts[4], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid source port: %s", parts[4])
	}
	destPort64, err := strconv.ParseUint(parts[5], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid destination port: %s", parts[5])
	}
	return &ProxyProtocolHeader{
		Protocol: proto,
		SrcIP:    srcIP,
		DestIP:   dstIP,
		SrcPort:  uint16(srcPort64),
		DestPort: uint16(destPort64),
	}, nil
}

func (h *ProxyProtocolHeader) ToBytes() ([]byte, error) {
	var builder strings.Builder
	_, err := fmt.Fprintf(&builder, "PROXY %s %s %s %d %d\r\n", h.Protocol, h.SrcIP, h.DestIP, h.SrcPort, h.DestPort)
	if err != nil {
		return nil, err
	}
	return []byte(builder.String()), nil
}
