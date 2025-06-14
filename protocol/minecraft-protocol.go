package protocol

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

type VarInt int32

type HandshakePacket struct {
	PacketID        VarInt
	ProtocolVersion VarInt
	ServerAddress   string
	ServerPort      uint16
	NextState       VarInt
}

func readVarInt(r io.ByteReader) (int32, error) {
	var numRead int
	var result int32
	for {
		if numRead >= 5 {
			return 0, fmt.Errorf("too many values read")
		}
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		value := b & 0x7F
		result |= int32(value) << (7 * numRead)
		numRead++
		if (b & 0x80) == 0 {
			break
		}
	}
	return result, nil
}

func encodeVarInt(v int32) []byte {
	var buf []byte
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if v == 0 {
			break
		}
	}
	return buf
}

func ParseHandshake(reader *bufio.Reader) (*HandshakePacket, []byte, error) {
	var data []byte
	// read packet len
	packetLen, err := readVarInt(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read packet length: %w", err)
	}
	// read full packet
	payload := make([]byte, packetLen)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, nil, fmt.Errorf("failed to read full packet: %w", err)
	}
	// prepare to parse handshake packet
	buf := bytes.NewReader(payload)
	// packet ID
	packetID, err := readVarInt(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read packet ID: %w", err)
	}
	// protocol version
	protoVer, err := readVarInt(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read protocol version: %w", err)
	}
	// server address
	addrLen, err := readVarInt(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read address length: %w", err)
	}
	addrBytes := make([]byte, addrLen)
	if _, err := io.ReadFull(buf, addrBytes); err != nil {
		return nil, nil, fmt.Errorf("failed to read address: %w", err)
	}
	serverAddr := string(addrBytes)
	// server port
	var serverPort uint16
	if err := binary.Read(buf, binary.BigEndian, &serverPort); err != nil {
		return nil, nil, fmt.Errorf("failed to read server port: %w", err)
	}
	// next state
	nextState, err := readVarInt(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read next state: %w", err)
	}

	h := &HandshakePacket{
		PacketID:        VarInt(packetID),
		ProtocolVersion: VarInt(protoVer),
		ServerAddress:   serverAddr,
		ServerPort:      serverPort,
		NextState:       VarInt(nextState),
	}

	data = append(encodeVarInt(packetLen), payload...)
	return h, data, nil
}
