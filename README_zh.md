# Minecraft Gateway

一个用 Go 编写的高性能 Minecraft 网关/代理服务器，根据握手包中的服务器地址将客户端连接路由到后端服务器。

[English](README.md)

## 功能特性

- **虚拟主机路由**：根据 Minecraft 握手包中的主机名将连接路由到不同的后端服务器
- **HAProxy PROXY 协议**：支持 v1 和 v2（接收 v1/v2，向上游发送 v1）
- **IP 白名单**：支持全局和服务器级别的 CIDR 访问控制
- **热重载**：无需重启即可重新加载配置
- **跨平台**：原生支持 Linux、macOS 和 Windows
- **单实例**：进程锁防止多实例运行

## 快速开始

### 构建

```bash
make build
```

### 运行

```bash
# 启动服务器
./bin/minecraft-gateway

# 重新加载配置
./bin/minecraft-gateway reload

# 停止服务器
./bin/minecraft-gateway stop
```

### Docker

```bash
# 构建镜像
docker build -t minecraft-gateway .

# 运行容器
docker run -p 25565:25565 -v ./config.yml:/srv/config.yml minecraft-gateway
```

## 配置

创建 `config.yml` 文件：

```yaml
timeout: 5s
listen_addr: ":25565"
default: "127.0.0.1:25577"

# 全局白名单（默认允许所有）
whitelist:
  - 0.0.0.0/0
  - "::/0"

# 全局 proxy protocol 设置
proxy_protocol:
  send_to_upstream: false
  receive_from_downstream: false

# 服务器列表
servers:
  - name: lobby.example.com
    address: "127.0.0.1:25578"
    # 可选：覆盖全局配置
    # whitelist:
    #   - 192.168.1.0/24
    # proxy_protocol:
    #   send_to_upstream: true

  - name: survival.example.com
    address: "127.0.0.1:25579"
```

### 配置选项

| 选项 | 描述 |
|------|------|
| `timeout` | 连接超时时间（如 `5s`、`10s`） |
| `listen_addr` | 监听地址（如 `:25565`） |
| `default` | 默认后端服务器地址 |
| `whitelist` | 全局 IP 白名单（CIDR 格式） |
| `proxy_protocol.send_to_upstream` | 向后端发送 PROXY 协议头 |
| `proxy_protocol.receive_from_downstream` | 期望从客户端接收 PROXY 协议 |
| `servers` | 虚拟主机映射列表 |

### 服务器选项

| 选项 | 描述 |
|------|------|
| `name` | 要匹配的主机名（来自 Minecraft 握手包） |
| `address` | 后端服务器地址 |
| `whitelist` | 可选：覆盖全局白名单 |
| `proxy_protocol` | 可选：覆盖全局 proxy protocol 设置 |

## 工作原理

1. 客户端连接到网关
2. 网关检查全局白名单
3. 网关解析 Minecraft 握手包以提取服务器地址
4. 网关检查服务器级别白名单（如果配置）
5. 网关连接到相应的后端服务器
6. 网关可选地向后端发送 PROXY 协议头
7. 网关双向转发流量

## 许可证

[MIT License](LICENSE)
