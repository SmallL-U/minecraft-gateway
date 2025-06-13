package gateway

import (
	"minecraft-gateway/config"
	"net"
	"sync"
)

type Gateway struct {
	Config      *config.Config
	configMutex sync.RWMutex
	listener    net.Listener
}

func NewGateway(cfg *config.Config) *Gateway {
	return &Gateway{Config: cfg}
}

func (g *Gateway) Start() error {
	return nil
}

func (g *Gateway) Stop() error {
	return nil
}
