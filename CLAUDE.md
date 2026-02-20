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
- **internal/logx/**: Structured logging wrapper around zap (global singleton initialized via `init()`, fixed at Info level)
- **internal/proc/**: Cross-platform process management (Unix: PID file at `/tmp/minecraft-gateway.pid`, Windows: Named Events)
- **internal/auth/**: Placeholder directory, currently empty

### Key Design Details

- **ParseHandshake** returns both the parsed `HandshakePacket` struct and the raw bytes of the original packet. The raw bytes are re-forwarded to the backend verbatim after optional PROXY protocol header injection.
- **Whitelist precedence**: A per-server whitelist entirely *replaces* the global whitelist (it does not extend it). `GetWhitelist()` returns server-specific or falls back to global.
- **Global whitelist check occurs before handshake parsing**; server-specific whitelist check occurs after parsing.
- **Config hot-reload** (`SIGHUP`) swaps the config pointer under `configMutex` — already-connected sessions continue unaffected; only new connections use the new config.

### Concurrency Architecture

- **Main Thread**: Handles signal processing (SIGINT/SIGTERM/SIGHUP) and configuration reloading
- **Accept Loop**: Single goroutine accepting incoming connections in `gateway.Start()`
- **Connection Handlers**: One goroutine per client connection in `handleConnection()`
- **Data Forwarding**: Two goroutines per connection (bidirectional data transfer via `io.Copy`)
- **Thread Safety**: `sync.RWMutex` protects config during hot reloads

### Connection Flow

1. Client connects → global whitelist check
2. Optional PROXY protocol v1/v2 parsing (if `receive_from_downstream` is enabled globally)
3. Minecraft handshake parsing → server-specific whitelist check
4. Backend selection based on `ServerAddress` from handshake (falls back to `default` if no match)
5. Backend connection establishment
6. Optional PROXY protocol v1 header injection (per-server configurable via `send_to_upstream`)
7. Original handshake bytes re-sent to backend, then bidirectional forwarding begins

## Development Commands

```bash
make build    # Build to bin/minecraft-gateway
make run      # Build and run
make reload   # Send reload signal to running instance (requires built binary)
make stop     # Send stop signal to running instance (requires built binary)
make clean    # Remove bin/ directory
make help     # Show all targets
```

There are currently no tests in this codebase.

## Configuration

Config is loaded from `config.yml` in the working directory. Validation requires `listen_addr`, `default`, and at least one server with non-empty `name` and `address`.

Whitelist entries accept both CIDR notation (`192.168.1.0/24`) and plain IPs (auto-converted to `/32` or `/128`).

```yaml
timeout: 5s
listen_addr: ":25565"
default: "127.0.0.1:25577"

whitelist:
  - 0.0.0.0/0
  - "::/0"

proxy_protocol:
  send_to_upstream: false
  receive_from_downstream: false

servers:
  - name: lobby.example.com
    address: "127.0.0.1:25578"
    # Per-server overrides (each replaces, not extends, the global setting)
    whitelist:
      - 192.168.1.0/24
    proxy_protocol:
      send_to_upstream: true
```

## Protocol Support

- **Minecraft Protocol**: Parses handshake packets (packet ID 0x00) to extract `ServerAddress` for routing; VarInt address length capped at 65535 bytes
- **HAProxy PROXY Protocol v1/v2**: Receives both v1 and v2 from downstream (via `go-proxyproto` library), sends only v1 to upstream
- **IPv4/IPv6**: Full support in both whitelist matching and PROXY protocol headers

## Signal Handling (Unix)

- **SIGINT/SIGTERM**: Graceful shutdown (closes listener, waits for in-flight connections)
- **SIGHUP**: Hot reload configuration
