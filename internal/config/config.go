package config

import (
	"fmt"
	"os"
	"time"

	"github.com/goccy/go-yaml"
)

type ProxyProtocolConfig struct {
	SendToUpstream        bool `yaml:"send_to_upstream"`
	ReceiveFromDownstream bool `yaml:"receive_from_downstream"`
}

type Config struct {
	Timeout       time.Duration       `yaml:"timeout"`
	ListenAddr    string              `yaml:"listen_addr"`
	Backends      map[string]string   `yaml:"backends"`
	Default       string              `yaml:"default"`
	ProxyProtocol ProxyProtocolConfig `yaml:"proxy_protocol"`
}

func validateConfig(config *Config) error {
	if config.ListenAddr == "" {
		return fmt.Errorf("listen address cannot be empty")
	}
	if len(config.Backends) == 0 {
		return fmt.Errorf("at least one backend must be defined")
	}
	for name, addr := range config.Backends {
		if name == "" || addr == "" {
			return fmt.Errorf("backend name and address cannot be empty: %s -> %s", name, addr)
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

	return config, nil
}

func Default() *Config {
	return &Config{
		Timeout:    5 * time.Second,
		ListenAddr: ":25565",
		Backends: map[string]string{
			"lobby.example.com":    "127.0.0.1:25578",
			"survival.example.com": "127.0.0.1:25579",
		},
		Default: "127.0.0.1:25577",
		ProxyProtocol: ProxyProtocolConfig{
			SendToUpstream:        false,
			ReceiveFromDownstream: false,
		},
	}
}
