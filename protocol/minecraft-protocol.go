package protocol

import "net"

type VarInt int32

type HandshakePacket struct {
	PacketID        VarInt
	ProtocolVersion VarInt
	ServerAddress   string
	ServerPort      uint16
	NextState       VarInt
}

func ParseHandshake(conn net.Conn) (*HandshakePacket, []byte, error) {
	// TODO: Implement the logic to read the handshake packet from the connection.
	return nil, nil, nil
}
