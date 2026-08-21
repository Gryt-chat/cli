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
// AdminPortBase is where management ports are searched from. A different range
// to the servers' own so the two are told apart at a glance in `docker ps` and
// in a firewall rule.
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

// portFree reports whether this machine will let a server bind the port.
//
// Bound on all interfaces on purpose: 0.0.0.0 is the default bind address, and
// a port free on loopback but held on another interface would still fail. On
// macOS that is exactly the AirPlay case.
//
// "tcp4", not "tcp", and the difference is not cosmetic. With "tcp" and an
// address of 0.0.0.0, Go opens a dual-stack socket that binds happily while
// something else already holds the IPv4 port — measured against a container
// publishing 0.0.0.0:5001, where "tcp" succeeded and "tcp4" correctly reported
// the address in use. Docker publishes on IPv4, so the probe has to ask about
// IPv4 or it hands out ports that are already taken and the start fails with
// "port is already allocated".
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
