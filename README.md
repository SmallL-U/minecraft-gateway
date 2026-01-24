# Minecraft Gateway

A high-performance Minecraft gateway/proxy server written in Go that routes client connections to backend servers based on the server address in the handshake packet.

[中文文档](README_zh.md)

## Features

- **Virtual Host Routing**: Route connections to different backend servers based on the hostname in Minecraft handshake
- **HAProxy PROXY Protocol**: Support for both v1 and v2 (receive v1/v2, send v1 to upstream)
- **IP Whitelist**: CIDR-based access control at global and per-server levels
- **Hot Reload**: Reload configuration without restarting the server
- **Cross-Platform**: Native support for Linux, macOS, and Windows
- **Single Instance**: Process lock to prevent multiple instances

## Quick Start

### Build

```bash
make build
```

### Run

```bash
# Start the server
./bin/minecraft-gateway

# Reload configuration
./bin/minecraft-gateway reload

# Stop the server
./bin/minecraft-gateway stop
```

### Docker

```bash
# Build image
docker build -t minecraft-gateway .

# Run container
docker run -p 25565:25565 -v ./config.yml:/srv/config.yml minecraft-gateway
```

## Configuration

Create a `config.yml` file:

```yaml
timeout: 5s
listen_addr: ":25565"
default: "127.0.0.1:25577"

# Global whitelist (allow all by default)
whitelist:
  - 0.0.0.0/0
  - "::/0"

# Global proxy protocol settings
proxy_protocol:
  send_to_upstream: false
  receive_from_downstream: false

# Server list
servers:
  - name: lobby.example.com
    address: "127.0.0.1:25578"
    # Optional: per-server overrides
    # whitelist:
    #   - 192.168.1.0/24
    # proxy_protocol:
    #   send_to_upstream: true

  - name: survival.example.com
    address: "127.0.0.1:25579"
```

### Configuration Options

| Option | Description |
|--------|-------------|
| `timeout` | Connection timeout (e.g., `5s`, `10s`) |
| `listen_addr` | Address to listen on (e.g., `:25565`) |
| `default` | Default backend server address |
| `whitelist` | Global IP whitelist (CIDR notation) |
| `proxy_protocol.send_to_upstream` | Send PROXY protocol header to backend |
| `proxy_protocol.receive_from_downstream` | Expect PROXY protocol from client |
| `servers` | List of virtual host mappings |

### Server Options

| Option | Description |
|--------|-------------|
| `name` | Hostname to match (from Minecraft handshake) |
| `address` | Backend server address |
| `whitelist` | Optional: Override global whitelist |
| `proxy_protocol` | Optional: Override global proxy protocol settings |

## How It Works

1. Client connects to the gateway
2. Gateway checks global whitelist
3. Gateway parses Minecraft handshake to extract server address
4. Gateway checks server-specific whitelist (if configured)
5. Gateway connects to the appropriate backend server
6. Gateway optionally sends PROXY protocol header to backend
7. Gateway forwards traffic bidirectionally

## License

[MIT License](LICENSE)
