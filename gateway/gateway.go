package gateway

import (
	"minecraft-gateway/config"
	"net"
	"sync"
)

type Gateway struct {
	config      *config.Config
	configMutex sync.RWMutex
	listener    net.Listener
}
