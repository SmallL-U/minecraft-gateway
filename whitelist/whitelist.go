package whitelist

import (
	"net"
	"strings"
	"sync"
)

type Whitelist struct {
	nets  []*net.IPNet
	mutex sync.RWMutex
}

func New(nets []*net.IPNet) *Whitelist {
	return &Whitelist{
		nets: nets,
	}
}

func Default() *Whitelist {
	return &Whitelist{
		nets: []*net.IPNet{
			{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},
			{IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)},
		},
	}
}

func ParseLines(lines []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		// ignore empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// if the line contains a CIDR notation, parse it
		if strings.Contains(line, "/") {
			if _, n, err := net.ParseCIDR(line); err == nil {
				nets = append(nets, n)
			}
			continue
		}
		ip := net.ParseIP(line)
		if ip == nil {
			continue
		}
		cidr := line + "/32"
		if ip.To4() == nil {
			cidr = line + "/128"
		}
		if _, n, err := net.ParseCIDR(cidr); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

func (w *Whitelist) ToLines() []string {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	lines := make([]string, 0, len(w.nets))
	for _, n := range w.nets {
		lines = append(lines, n.String())
	}
	println(lines)
	return lines
}

func (w *Whitelist) Update(nets []*net.IPNet) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.nets = nets
}

func (w *Whitelist) Allowed(ip net.IP) bool {
	if ip == nil {
		return false
	}
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	for _, n := range w.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
