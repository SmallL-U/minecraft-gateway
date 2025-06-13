package protocol

import "net"

type ProxyProtocolHeader struct {
	Protocol string
	SrcIP    string
	DestIP   string
	SrcPort  uint16
	DestPort uint16
}

func ParseProxyProtocol(conn net.Conn) (*ProxyProtocolHeader, error) {
	// TODO: Implement the logic to read the proxy protocol header from the connection.
	return nil, nil
}
