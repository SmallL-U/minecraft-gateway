# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Minecraft gateway/proxy server written in Go that routes connections between clients and backend Minecraft servers based on the server address in the handshake packet. It supports both standard TCP proxying and HAProxy PROXY protocol (v1/v2) for preserving client IP addresses.

## Architecture

The application follows a standard Go project layout:

- **cmd/minecraft-gateway/**: Entry point with configuration loading, signal handling, and graceful shutdown
- **internal/gateway/**: Core proxy logic with connection handling and data forwarding
- **internal/config/**: Configuration management with YAML loading, validation, and whitelist parsing
- **internal/protocol/**: Minecraft and PROXY protocol parsers (supports v1 and v2)
- **internal/logx/**: Structured logging wrapper around zap
- **internal/pidfile/**: PID file management for single instance enforcement and signal handling

### Key Components

- **Gateway**: Main proxy server that listens for connections, parses Minecraft handshakes, selects backends based on server address, and forwards traffic
- **Config**: Hot-reloadable configuration supporting server list with per-server whitelist and proxy protocol overrides
- **Protocol Parsers**: Handle Minecraft handshake parsing and HAProxy PROXY protocol headers (v1/v2 input, v1 output)

### Concurrency Architecture

The gateway uses a multi-goroutine architecture with proper synchronization:

- **Main Thread**: Handles signal processing (SIGINT/SIGTERM/SIGHUP) and configuration reloading
- **Accept Loop**: Single goroutine accepting incoming connections in `gateway.Start()`
- **Connection Handlers**: One goroutine per client connection in `handleConnection()`
- **Data Forwarding**: Two goroutines per connection (bidirectional data transfer)
- **Thread Safety**: RWMutex protects config during hot reloads

### Connection Flow

1. Client connects → global whitelist check
2. Handshake parsing → server-specific whitelist check
3. Backend selection based on server address from handshake
4. Backend connection establishment
5. Optional PROXY protocol header injection (per-server configurable)
6. Bidirectional data forwarding with proper connection cleanup

## Development Commands

### Building
```bash
make build
```

### Running
```bash
make run
```

### Docker
```bash
# Build image
docker build -t minecraft-gateway .

# Run container
docker run -p 25565:25565 minecraft-gateway
```

### Configuration Files

- **config.yml**: Main configuration with listen address, server list, global/per-server whitelist and PROXY protocol settings
- **minecraft-gateway.pid**: PID file for single instance enforcement

### Hot Reload

Reload configuration without restart:
```bash
make reload
# or
./bin/minecraft-gateway reload
```

### Stop Server

Stop the running instance gracefully:
```bash
make stop
# or
./bin/minecraft-gateway stop
```

## Configuration

The configuration supports global settings with per-server overrides:

```yaml
timeout: 5s
listen_addr: ":25565"
default: "127.0.0.1:25577"

# Global whitelist
whitelist:
  - 0.0.0.0/0
  - "::/0"

# Global proxy protocol
proxy_protocol:
  send_to_upstream: false
  receive_from_downstream: false

# Server list
servers:
  - name: lobby.example.com
    address: "127.0.0.1:25578"
    # Optional per-server overrides
    whitelist:
      - 192.168.1.0/24
    proxy_protocol:
      send_to_upstream: true
```

## Protocol Support

- **Minecraft Protocol**: Parses handshake packets to extract server address for routing
- **HAProxy PROXY Protocol v1/v2**: Receives both v1 and v2 from downstream, sends v1 to upstream
- **IPv4/IPv6**: Full support for both IP versions

## Security Considerations

- **VarInt Validation**: Address length limited to 65535 bytes to prevent memory exhaustion attacks
- **Type Safety**: Safe type assertions with proper error handling to prevent panics
- **Resource Management**: Proper connection cleanup and goroutine lifecycle management
- **Whitelist Enforcement**: CIDR-based IP filtering at global and per-server levels

## Signal Handling

- **SIGINT/SIGTERM**: Graceful shutdown
- **SIGHUP**: Hot reload configuration
