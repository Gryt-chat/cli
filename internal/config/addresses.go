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

// LocalAddresses lists the IPv4 addresses of this machine's up interfaces,
// loopback excluded.
//
// Nothing is asked of the network to produce this: it reads the interfaces and
// stops. A machine behind NAT therefore reports its private address and not the
// address the internet sees, which is the honest answer. Finding the latter
// means asking a third party what it thinks your address is, and that is a
// request Gryt should not make on somebody's behalf without being told to.
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

// isVirtual drops the interfaces that exist because of software on this
// machine rather than because of a network somebody can reach it over.
//
// Not cosmetic. A Mac running Docker reported seven addresses, five of them
// bridges to container networks; advertising those in ICE gives every client a
// handful of candidates that can never connect and makes them wait to find out.
// The list is names rather than address ranges because a Docker bridge and a
// home network both look like 192.168.
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

// AdvertiseIPs is what the SFU should announce in ICE candidates: every
// address this machine answers on.
//
// It belongs to the machine rather than to any one server, which is why it is
// derived here and written into the shared project instead of being asked
// about per server. The SFU takes a comma-separated list and clients pick
// whichever path works.
func AdvertiseIPs() string {
	addresses := LocalAddresses()
	ips := make([]string, 0, len(addresses))
	for _, address := range addresses {
		ips = append(ips, address.IP)
	}
	return strings.Join(ips, ",")
}
