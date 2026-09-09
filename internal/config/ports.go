package config

import (
	"net"
	"strconv"
)

// DefaultPort is where the search starts. It is the port the deployment docs
// use, so a machine with nothing in the way still gets the documented one.
const DefaultPort = 5000

// FreePort returns the first port at or above DefaultPort that nothing holds and no existing
// server claims. On macOS 5000 is ControlCenter's AirPlay receiver, so a fixed default lied.

// AdminPortBase is where management ports are searched from — a different range to the
// servers' own, so the two are told apart in `docker ps` and in a firewall rule.
const AdminPortBase = 5090

// FreeAdminPort returns a management port nothing holds and no other server
// claims.
func FreeAdminPort(taken []int) int {
	return freePortFrom(AdminPortBase, taken)
}

func FreePort(taken []int) int {
	return freePortFrom(DefaultPort, taken)
}

func freePortFrom(start int, taken []int) int {
	claimed := make(map[int]bool, len(taken))
	for _, port := range taken {
		claimed[port] = true
	}
	for port := start; port < start+200; port++ {
		if claimed[port] || !portFree(port) {
			continue
		}
		return port
	}
	// Nothing free in a 200-port window is not a situation to guess about.
	return start
}

// portFree reports whether this machine will let a server bind the port, on all interfaces.
// "tcp4", not "tcp": a dual-stack socket binds happily while Docker holds the IPv4 port.
func portFree(port int) bool {
	listener, err := net.Listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

// PortsInUse returns every port these profiles claim, both the server's and
// its management port, so a search for either skips all of them.
func PortsInUse(profiles []Profile) []int {
	ports := make([]int, 0, len(profiles)*2)
	for _, profile := range profiles {
		ports = append(ports, profile.Port)
		if profile.AdminPort > 0 {
			ports = append(ports, profile.AdminPort)
		}
	}
	return ports
}
