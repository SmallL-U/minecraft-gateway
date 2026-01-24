package config

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/goccy/go-yaml"
)

type ProxyProtocolConfig struct {
	SendToUpstream        bool `yaml:"send_to_upstream"`
	ReceiveFromDownstream bool `yaml:"receive_from_downstream"`
}

type Server struct {
	Name          string               `yaml:"name"`
	Address       string               `yaml:"address"`
	Whitelist     []string             `yaml:"whitelist,omitempty"`
	ProxyProtocol *ProxyProtocolConfig `yaml:"proxy_protocol,omitempty"`
}

type Config struct {
	Timeout       time.Duration       `yaml:"timeout"`
	ListenAddr    string              `yaml:"listen_addr"`
	Default       string              `yaml:"default"`
	Whitelist     []string            `yaml:"whitelist"`
	ProxyProtocol ProxyProtocolConfig `yaml:"proxy_protocol"`
	Servers       []Server            `yaml:"servers"`

	// Parsed whitelist networks (populated after loading)
	globalWhitelist  []*net.IPNet
	serverWhitelists map[string][]*net.IPNet
}

func parseWhitelist(entries []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, entry := range entries {
		if entry == "" {
			continue
		}
		// Try parsing as CIDR
		_, ipNet, err := net.ParseCIDR(entry)
		if err == nil {
			nets = append(nets, ipNet)
			continue
		}
		// Try parsing as IP
		ip := net.ParseIP(entry)
		if ip == nil {
			continue
		}
		// Convert IP to CIDR
		if ip.To4() != nil {
			_, ipNet, _ = net.ParseCIDR(entry + "/32")
		} else {
			_, ipNet, _ = net.ParseCIDR(entry + "/128")
		}
		if ipNet != nil {
			nets = append(nets, ipNet)
		}
	}
	return nets
}

func (c *Config) parseWhitelists() {
	c.globalWhitelist = parseWhitelist(c.Whitelist)
	c.serverWhitelists = make(map[string][]*net.IPNet)
	for _, server := range c.Servers {
		if len(server.Whitelist) > 0 {
			c.serverWhitelists[server.Name] = parseWhitelist(server.Whitelist)
		}
	}
}

// GetWhitelist returns the whitelist for the given server name, or global whitelist if not specified.
func (c *Config) GetWhitelist(serverName string) []*net.IPNet {
	if wl, ok := c.serverWhitelists[serverName]; ok {
		return wl
	}
	return c.globalWhitelist
}

// GetProxyProtocol returns the proxy protocol config for the given server name, or global config if not specified.
func (c *Config) GetProxyProtocol(serverName string) ProxyProtocolConfig {
	for _, server := range c.Servers {
		if server.Name == serverName && server.ProxyProtocol != nil {
			return *server.ProxyProtocol
		}
	}
	return c.ProxyProtocol
}

// GetServerAddress returns the backend address for the given server name.
func (c *Config) GetServerAddress(serverName string) string {
	for _, server := range c.Servers {
		if server.Name == serverName {
			return server.Address
		}
	}
	return c.Default
}

// IsAllowed checks if the IP is allowed by the whitelist for the given server.
func (c *Config) IsAllowed(serverName string, ip net.IP) bool {
	if ip == nil {
		return false
	}
	whitelist := c.GetWhitelist(serverName)
	for _, ipNet := range whitelist {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// IsAllowedByGlobal checks if the IP is allowed by the global whitelist.
func (c *Config) IsAllowedByGlobal(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, ipNet := range c.globalWhitelist {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

func validateConfig(config *Config) error {
	if config.ListenAddr == "" {
		return fmt.Errorf("listen address cannot be empty")
	}
	if len(config.Servers) == 0 {
		return fmt.Errorf("at least one server must be defined")
	}
	for _, server := range config.Servers {
		if server.Name == "" {
			return fmt.Errorf("server name cannot be empty")
		}
		if server.Address == "" {
			return fmt.Errorf("server address cannot be empty for server: %s", server.Name)
		}
	}
	if config.Default == "" {
		return fmt.Errorf("default backend address cannot be empty")
	}
	return nil
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("error decoding config from YAML: %v", err)
	}

	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %v", err)
	}

	config.parseWhitelists()

	return config, nil
}
