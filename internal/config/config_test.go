package config

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
	return path
}

const validConfigYAML = `
timeout: 10s
listen_addr: ":25565"
default: "127.0.0.1:25577"
log_level: debug

whitelist:
  - 0.0.0.0/0
  - "::/0"

proxy_protocol:
  send_to_upstream: false
  receive_from_downstream: false

servers:
  - name: lobby.example.com
    address: "127.0.0.1:25578"
    whitelist:
      - 192.168.1.0/24
    proxy_protocol:
      send_to_upstream: true
`

func TestLoadConfig_Valid(t *testing.T) {
	path := writeConfigFile(t, validConfigYAML)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 10*time.Second)
	}
	if cfg.ListenAddr != ":25565" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":25565")
	}
	if cfg.Default != "127.0.0.1:25577" {
		t.Errorf("Default = %q, want %q", cfg.Default, "127.0.0.1:25577")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("len(Servers) = %d, want 1", len(cfg.Servers))
	}
	if cfg.Servers[0].Name != "lobby.example.com" {
		t.Errorf("Servers[0].Name = %q, want %q", cfg.Servers[0].Name, "lobby.example.com")
	}
	if cfg.Servers[0].Address != "127.0.0.1:25578" {
		t.Errorf("Servers[0].Address = %q, want %q", cfg.Servers[0].Address, "127.0.0.1:25578")
	}
}

func TestLoadConfig_FileNotExist(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "does-not-exist.yml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want errors.Is(err, os.ErrNotExist) to be true", err)
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	path := writeConfigFile(t, "listen_addr: [this is not\n  valid: yaml::")
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoadConfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "missing listen_addr",
			yaml: `
default: "127.0.0.1:25577"
servers:
  - name: lobby
    address: "127.0.0.1:25578"
`,
		},
		{
			name: "missing default",
			yaml: `
listen_addr: ":25565"
servers:
  - name: lobby
    address: "127.0.0.1:25578"
`,
		},
		{
			name: "empty servers",
			yaml: `
listen_addr: ":25565"
default: "127.0.0.1:25577"
servers: []
`,
		},
		{
			name: "server missing name",
			yaml: `
listen_addr: ":25565"
default: "127.0.0.1:25577"
servers:
  - name: ""
    address: "127.0.0.1:25578"
`,
		},
		{
			name: "server missing address",
			yaml: `
listen_addr: ":25565"
default: "127.0.0.1:25577"
servers:
  - name: lobby
    address: ""
`,
		},
		{
			name: "invalid log_level",
			yaml: `
listen_addr: ":25565"
default: "127.0.0.1:25577"
log_level: nonsense
servers:
  - name: lobby
    address: "127.0.0.1:25578"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfigFile(t, tt.yaml)
			_, err := LoadConfig(path)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestLoadConfig_DuplicateServerName(t *testing.T) {
	yaml := `
listen_addr: ":25565"
default: "127.0.0.1:25577"
servers:
  - name: lobby
    address: "127.0.0.1:25578"
  - name: lobby
    address: "127.0.0.1:25579"
`
	path := writeConfigFile(t, yaml)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for duplicate server name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate server name: lobby") {
		t.Errorf("error = %q, want duplicate server name", err)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	yaml := `
listen_addr: ":25565"
default: "127.0.0.1:25577"
servers:
  - name: lobby
    address: "127.0.0.1:25578"
`
	path := writeConfigFile(t, yaml)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Timeout != defaultTimeout {
		t.Errorf("Timeout = %v, want default %v", cfg.Timeout, defaultTimeout)
	}
	if cfg.LogLevel != defaultLogLevel {
		t.Errorf("LogLevel = %q, want default %q", cfg.LogLevel, defaultLogLevel)
	}
}

func TestLoadConfig_LogLevelWarningNormalized(t *testing.T) {
	yaml := `
listen_addr: ":25565"
default: "127.0.0.1:25577"
log_level: WARNING
servers:
  - name: lobby
    address: "127.0.0.1:25578"
`
	path := writeConfigFile(t, yaml)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "warn")
	}
}

func TestParseWhitelist(t *testing.T) {
	entries := []string{
		"192.168.1.0/24",
		"1.2.3.4",
		"::1",
		"",
		"not-an-ip-or-cidr",
	}

	nets := parseWhitelist(entries)

	if len(nets) != 3 {
		t.Fatalf("len(nets) = %d, want 3; nets=%v", len(nets), nets)
	}

	if nets[0].String() != "192.168.1.0/24" {
		t.Errorf("nets[0] = %q, want %q", nets[0].String(), "192.168.1.0/24")
	}
	if !nets[1].Contains(net.ParseIP("1.2.3.4")) {
		t.Errorf("nets[1] does not contain 1.2.3.4: %q", nets[1].String())
	}
	ones, bits := nets[1].Mask.Size()
	if ones != 32 || bits != 32 {
		t.Errorf("nets[1] mask = /%d (bits=%d), want /32", ones, bits)
	}
	if !nets[2].Contains(net.ParseIP("::1")) {
		t.Errorf("nets[2] does not contain ::1: %q", nets[2].String())
	}
	ones, bits = nets[2].Mask.Size()
	if ones != 128 || bits != 128 {
		t.Errorf("nets[2] mask = /%d (bits=%d), want /128", ones, bits)
	}
}

func TestIsAllowedByGlobal(t *testing.T) {
	cfg := &Config{Whitelist: []string{"192.168.1.0/24"}}
	cfg.parseWhitelists()

	tests := []struct {
		name string
		ip   net.IP
		want bool
	}{
		{"allowed", net.ParseIP("192.168.1.42"), true},
		{"not allowed", net.ParseIP("10.0.0.1"), false},
		{"nil ip", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.IsAllowedByGlobal(tt.ip); got != tt.want {
				t.Errorf("IsAllowedByGlobal(%v) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestIsAllowed(t *testing.T) {
	cfg := &Config{
		Whitelist: []string{"10.0.0.0/8"},
		Servers: []Server{
			{Name: "lobby", Address: "127.0.0.1:25578", Whitelist: []string{"192.168.1.0/24"}},
		},
	}
	cfg.parseWhitelists()

	tests := []struct {
		name       string
		serverName string
		ip         net.IP
		want       bool
	}{
		{"server whitelist hit", "lobby", net.ParseIP("192.168.1.5"), true},
		{"server whitelist miss (global would match)", "lobby", net.ParseIP("10.1.2.3"), false},
		{"fallback to global hit", "unknown-server", net.ParseIP("10.1.2.3"), true},
		{"fallback to global miss", "unknown-server", net.ParseIP("172.16.0.1"), false},
		{"nil ip", "lobby", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.IsAllowed(tt.serverName, tt.ip); got != tt.want {
				t.Errorf("IsAllowed(%q, %v) = %v, want %v", tt.serverName, tt.ip, got, tt.want)
			}
		})
	}
}

func TestGetWhitelist(t *testing.T) {
	cfg := &Config{
		Whitelist: []string{"0.0.0.0/0"},
		Servers: []Server{
			{Name: "with-override", Address: "127.0.0.1:1", Whitelist: []string{"192.168.1.0/24"}},
			{Name: "without-override", Address: "127.0.0.1:2"},
		},
	}
	cfg.parseWhitelists()

	t.Run("server override replaces global", func(t *testing.T) {
		wl := cfg.GetWhitelist("with-override")
		if len(wl) != 1 {
			t.Fatalf("len(wl) = %d, want 1", len(wl))
		}
		if wl[0].String() != "192.168.1.0/24" {
			t.Errorf("wl[0] = %q, want %q", wl[0].String(), "192.168.1.0/24")
		}
	})

	t.Run("no override falls back to global", func(t *testing.T) {
		wl := cfg.GetWhitelist("without-override")
		if len(wl) != 1 {
			t.Fatalf("len(wl) = %d, want 1", len(wl))
		}
		if wl[0].String() != "0.0.0.0/0" {
			t.Errorf("wl[0] = %q, want %q", wl[0].String(), "0.0.0.0/0")
		}
	})

	t.Run("unknown server falls back to global", func(t *testing.T) {
		wl := cfg.GetWhitelist("nonexistent")
		if len(wl) != 1 || wl[0].String() != "0.0.0.0/0" {
			t.Errorf("GetWhitelist(nonexistent) = %v, want global whitelist", wl)
		}
	})
}

func TestGetServerAddress(t *testing.T) {
	cfg := &Config{
		Default: "127.0.0.1:25577",
		Servers: []Server{
			{Name: "lobby.example.com", Address: "127.0.0.1:25578"},
		},
	}

	tests := []struct {
		name       string
		serverName string
		want       string
	}{
		{"matching server", "lobby.example.com", "127.0.0.1:25578"},
		{"unknown server falls back to default", "unknown.example.com", "127.0.0.1:25577"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.GetServerAddress(tt.serverName); got != tt.want {
				t.Errorf("GetServerAddress(%q) = %q, want %q", tt.serverName, got, tt.want)
			}
		})
	}
}

func TestGetProxyProtocol(t *testing.T) {
	globalPP := ProxyProtocolConfig{SendToUpstream: false, ReceiveFromDownstream: true}
	serverPP := ProxyProtocolConfig{SendToUpstream: true, ReceiveFromDownstream: false}

	cfg := &Config{
		ProxyProtocol: globalPP,
		Servers: []Server{
			{Name: "with-pp", Address: "127.0.0.1:1", ProxyProtocol: &serverPP},
			{Name: "without-pp", Address: "127.0.0.1:2"},
		},
	}

	t.Run("server-level config returned when present", func(t *testing.T) {
		got := cfg.GetProxyProtocol("with-pp")
		if got != serverPP {
			t.Errorf("GetProxyProtocol(with-pp) = %+v, want %+v", got, serverPP)
		}
	})

	t.Run("falls back to global when server has no override", func(t *testing.T) {
		got := cfg.GetProxyProtocol("without-pp")
		if got != globalPP {
			t.Errorf("GetProxyProtocol(without-pp) = %+v, want %+v", got, globalPP)
		}
	})

	t.Run("falls back to global for unknown server", func(t *testing.T) {
		got := cfg.GetProxyProtocol("unknown")
		if got != globalPP {
			t.Errorf("GetProxyProtocol(unknown) = %+v, want %+v", got, globalPP)
		}
	})
}
