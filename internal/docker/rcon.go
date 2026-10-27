package docker

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"

	"github.com/docker/go-connections/nat"
)

// RCONAddresses discovers the actual RCON TCP binding or private container
// address. It never assumes the game port also speaks RCON and publishes no ports.
func (m *Manager) RCONAddresses(ctx context.Context, containerID string, port int) ([]string, error) {
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid RCON port")
	}
	inspect, err := m.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect RCON connection: %w", err)
	}
	addresses := []string{}
	seen := map[string]bool{}
	add := func(host, port string) {
		if host == "" || port == "" {
			return
		}
		address := net.JoinHostPort(host, port)
		if !seen[address] {
			addresses = append(addresses, address)
			seen[address] = true
		}
	}
	settings := inspect.NetworkSettings
	if settings != nil {
		key := nat.Port(strconv.Itoa(port) + "/tcp")
		for _, binding := range settings.Ports[key] {
			host := binding.HostIP
			if ip := net.ParseIP(host); host == "" || ip != nil && (ip.IsUnspecified() || ip.IsLoopback()) {
				host = m.HostAddress()
			}
			add(host, binding.HostPort)
		}
		if endpoint := settings.Networks[NetworkName]; endpoint != nil {
			add(endpoint.IPAddress, strconv.Itoa(port))
		}
		add(settings.IPAddress, strconv.Itoa(port))
		names := make([]string, 0, len(settings.Networks))
		for name := range settings.Networks {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if endpoint := settings.Networks[name]; endpoint != nil {
				add(endpoint.IPAddress, strconv.Itoa(port))
			}
		}
	}
	if inspect.ContainerJSONBase != nil && inspect.HostConfig != nil && inspect.HostConfig.NetworkMode.IsHost() {
		add(m.HostAddress(), strconv.Itoa(port))
	}
	return addresses, nil
}
