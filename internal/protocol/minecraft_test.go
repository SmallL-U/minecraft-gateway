package protocol

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"testing"
)

func TestVarIntRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		value int32
	}{
		{"zero", 0},
		{"one", 1},
		{"127 (single byte boundary)", 127},
		{"128 (two byte boundary)", 128},
		{"300", 300},
		{"large value", 2147483647},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := encodeVarInt(tt.value)
			got, err := readVarInt(bytes.NewReader(encoded))
			if err != nil {
				t.Fatalf("readVarInt() unexpected error: %v", err)
			}
			if got != tt.value {
				t.Errorf("readVarInt(encodeVarInt(%d)) = %d, want %d", tt.value, got, tt.value)
			}
		})
	}
}

func TestReadVarInt_TooManyBytes(t *testing.T) {
	// Five bytes, each with the continuation bit set, forces a sixth read
	// attempt which must fail with "too many values read".
	data := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	_, err := readVarInt(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for VarInt longer than 5 bytes, got nil")
	}
}

// buildHandshakePayload assembles the inner payload of a handshake packet:
// packet ID (0x00) + protocol version + address length + address + port + next state.
func buildHandshakePayload(protocolVersion int32, address string, port uint16, nextState int32) []byte {
	var payload []byte
	payload = append(payload, encodeVarInt(0)...) // packet ID: handshake
	payload = append(payload, encodeVarInt(protocolVersion)...)
	payload = append(payload, encodeVarInt(int32(len(address)))...)
	payload = append(payload, []byte(address)...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, port)
	payload = append(payload, portBytes...)
	payload = append(payload, encodeVarInt(nextState)...)
	return payload
}

func buildHandshakePacket(protocolVersion int32, address string, port uint16, nextState int32) []byte {
	payload := buildHandshakePayload(protocolVersion, address, port, nextState)
	return append(encodeVarInt(int32(len(payload))), payload...)
}

func TestParseHandshake_Valid(t *testing.T) {
	tests := []struct {
		name            string
		protocolVersion int32
		address         string
		port            uint16
		nextState       int32
	}{
		{"status ping", 47, "play.example.com", 25565, 1},
		{"login", 763, "192.168.1.10", 25577, 2},
		{"empty address", 0, "", 0, 1},
		{"srv record style address", 47, "example.com\x00192.168.1.1\x0025565", 25565, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := buildHandshakePacket(tt.protocolVersion, tt.address, tt.port, tt.nextState)
			reader := bufio.NewReader(bytes.NewReader(raw))

			h, data, err := ParseHandshake(reader)
			if err != nil {
				t.Fatalf("ParseHandshake() unexpected error: %v", err)
			}

			if h.PacketID != 0 {
				t.Errorf("PacketID = %d, want 0", h.PacketID)
			}
			if h.ProtocolVersion != VarInt(tt.protocolVersion) {
				t.Errorf("ProtocolVersion = %d, want %d", h.ProtocolVersion, tt.protocolVersion)
			}
			if h.ServerAddress != tt.address {
				t.Errorf("ServerAddress = %q, want %q", h.ServerAddress, tt.address)
			}
			if h.ServerPort != tt.port {
				t.Errorf("ServerPort = %d, want %d", h.ServerPort, tt.port)
			}
			if h.NextState != VarInt(tt.nextState) {
				t.Errorf("NextState = %d, want %d", h.NextState, tt.nextState)
			}

			// The raw bytes returned must exactly match the original input,
			// since they are re-forwarded verbatim to the backend.
			if !bytes.Equal(data, raw) {
				t.Errorf("returned raw bytes = %x, want %x", data, raw)
			}
		})
	}
}

func TestParseHandshake_Errors(t *testing.T) {
	validPayload := buildHandshakePayload(47, "example.com", 25565, 1)
	validPacket := append(encodeVarInt(int32(len(validPayload))), validPayload...)

	oversizedAddrPayload := append(encodeVarInt(0), encodeVarInt(47)...)
	oversizedAddrPayload = append(oversizedAddrPayload, encodeVarInt(65536)...)
	oversizedAddrPacket := append(encodeVarInt(int32(len(oversizedAddrPayload))), oversizedAddrPayload...)

	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "truncated packet (missing trailing bytes)",
			data: validPacket[:len(validPacket)-5],
		},
		{
			name: "empty input",
			data: []byte{},
		},
		{
			name: "address length exceeds 65535",
			data: oversizedAddrPacket,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(bytes.NewReader(tt.data))
			_, _, err := ParseHandshake(reader)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
