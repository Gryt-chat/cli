package config

import (
	"net"
	"strconv"
)

// DefaultPort is where the search starts. It is the port the deployment docs
// use, so a machine with nothing in the way still gets the documented one.
const DefaultPort = 5000

// FreePort returns the first port at or above DefaultPort that nothing holds
// and no existing server claims.
//
// A fixed default was wrong twice over. Every new server got 5000, so the
// second one on a machine failed to bind. And on macOS 5000 is taken before
// anything else starts: ControlCenter listens there for AirPlay Receiver, so
// the server bound nothing the host could reach, the dashboard showed it as
// unknown, and anybody given the address reached an AirPlay receiver instead.
//
// Probing rather than hardcoding a different number means this keeps working
// when the next thing squats the next port.
func FreePort(taken []int) int {
	claimed := make(map[int]bool, len(taken))
	for _, port := range taken {
		claimed[port] = true
	}
	for port := DefaultPort; port < DefaultPort+200; port++ {
		if claimed[port] || !portFree(port) {
			continue
		}
		return port
	}
	// Nothing free in a 200-port window is not a situation to guess about.
	return DefaultPort
}

// portFree reports whether this machine will let a server bind the port.
//
// Bound on all interfaces on purpose: 0.0.0.0 is the default bind address, and
// a port free on loopback but held on another interface would still fail. On
// macOS that is exactly the AirPlay case.
func portFree(port int) bool {
	listener, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

// PortsInUse returns the ports these profiles claim, so a search can skip them.
func PortsInUse(profiles []Profile) []int {
	ports := make([]int, 0, len(profiles))
	for _, profile := range profiles {
		ports = append(ports, profile.Port)
	}
	return ports
}
