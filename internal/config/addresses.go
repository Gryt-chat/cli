package config

import (
	"net"
	"sort"
	"strings"
)

// Address is one way this machine can be reached.
type Address struct {
	IP    string
	Label string
}

// LocalAddresses lists the IPv4 addresses of this machine's up interfaces, loopback excluded.
// Nothing is asked of the network, so a machine behind NAT reports its private address.
func LocalAddresses() []Address {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var found []Address
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtual(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			// IPv4 only. The SFU advertises both, but a checkbox list of
			// link-local IPv6 addresses is a worse question than no question.
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			found = append(found, Address{IP: ip.String(), Label: labelFor(ip, iface.Name)})
		}
	}

	sort.Slice(found, func(i, j int) bool { return found[i].IP < found[j].IP })
	return found
}

// isVirtual drops interfaces that exist because of software rather than a reachable network.
// Names rather than ranges: a Docker bridge and a home network both look like 192.168.
func isVirtual(name string) bool {
	prefixes := []string{
		"bridge", "vmnet", "utun", "awdl", "llw", "ap", "anpi", // macOS
		"docker", "br-", "veth", "virbr", "tun", "tap", "cni", // Linux
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func labelFor(ip net.IP, iface string) string {
	if ip.IsPrivate() {
		return iface + ", local network"
	}
	return iface + ", reachable from the internet"
}

// AdvertiseIPs is what the SFU should announce in ICE candidates. It belongs to the machine
// rather than any one server, so it is written into the shared project.
func AdvertiseIPs() string {
	addresses := LocalAddresses()
	ips := make([]string, 0, len(addresses))
	for _, address := range addresses {
		ips = append(ips, address.IP)
	}
	return strings.Join(ips, ",")
}
