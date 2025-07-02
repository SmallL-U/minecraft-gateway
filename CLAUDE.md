# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Minecraft gateway/proxy server written in Go that routes connections between clients and backend Minecraft servers based on the server address in the handshake packet. It supports both standard TCP proxying and HAProxy PROXY protocol for preserving client IP addresses.

## Architecture

The application follows a modular architecture:

- **main.go**: Entry point with configuration loading, signal handling, and graceful shutdown
- **gateway/**: Core proxy logic with connection handling and data forwarding  
- **config/**: Configuration management with JSON loading and validation
- **protocol/**: Minecraft and PROXY protocol parsers
- **whitelist/**: IP-based access control using CIDR ranges
- **logx/**: Structured logging wrapper around zap
- **util/**: File I/O utilities for JSON and line-based files

### Key Components

- **Gateway**: Main proxy server that listens for connections, parses Minecraft handshakes, selects backends based on server address, and forwards traffic
- **Config**: Hot-reloadable configuration supporting multiple backend mappings and PROXY protocol settings
- **Whitelist**: CIDR-based IP filtering that can be reloaded without restart
- **Protocol Parsers**: Handle Minecraft handshake parsing and HAProxy PROXY protocol headers

### Concurrency Architecture

The gateway uses a multi-goroutine architecture with proper synchronization:

- **Main Thread**: Handles signal processing (SIGINT/SIGTERM/SIGHUP) and configuration reloading
- **Accept Loop**: Single goroutine accepting incoming connections in `gateway.Start()`
- **Connection Handlers**: One goroutine per client connection in `handleConnection()`
- **Data Forwarding**: Two goroutines per connection (bidirectional data transfer)
- **Thread Safety**: RWMutex protects config and whitelist during hot reloads

### Connection Flow

1. Client connects → whitelist check → handshake parsing
2. Backend selection based on server address from handshake
3. Backend connection establishment
4. Optional PROXY protocol header injection
5. Bidirectional data forwarding with proper connection cleanup

## Development Commands

### Building
```bash
go build -o minecraft-gateway main.go
```

### Running
```bash
# Generate default config (exits after creation)
go run main.go

# Run with existing config
go run main.go
```

### Docker
```bash
# Build image
docker build -t minecraft-gateway .

# Run container
docker run -p 25565:25565 minecraft-gateway
```

### Configuration Files

- **config.json**: Main configuration with listen address, backend mappings, timeouts, and PROXY protocol settings
- **whitelist.txt**: CIDR ranges for IP-based access control, one per line

### Hot Reload

Send SIGHUP to reload configuration and whitelist without restart:
```bash
kill -HUP <pid>
```

## Protocol Support

- **Minecraft Protocol**: Parses handshake packets to extract server address for routing
- **HAProxy PROXY Protocol v1**: Preserves original client IP when behind load balancers
- **IPv4/IPv6**: Full support for both IP versions

## Security Considerations

- **VarInt Validation**: Address length limited to 65535 bytes to prevent memory exhaustion attacks
- **Type Safety**: Safe type assertions with proper error handling to prevent panics
- **Resource Management**: Proper connection cleanup and goroutine lifecycle management
- **Whitelist Enforcement**: IP-based access control with CIDR support

## Signal Handling

- **SIGINT/SIGTERM**: Graceful shutdown
- **SIGHUP**: Hot reload configuration and whitelist