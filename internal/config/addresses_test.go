package config

import (
	"os"
	"strings"
	"testing"
)

func TestVirtualInterfacesAreExcluded(t *testing.T) {
	virtual := []string{"bridge100", "docker0", "vmnet1", "utun4", "awdl0", "br-2f1a", "veth9c", "virbr0"}
	for _, name := range virtual {
		if !isVirtual(name) {
			t.Errorf("%s should be treated as virtual", name)
		}
	}
	real := []string{"en0", "en9", "eth0", "wlan0", "enp3s0", "ens18"}
	for _, name := range real {
		if isVirtual(name) {
			t.Errorf("%s is a real interface and should be kept", name)
		}
	}
}

// The reason the filter exists: a Mac running Docker reported seven addresses,
// five of them bridges to container networks. Advertising those hands every
// client candidates that can never connect.
func TestLocalAddressesSkipsLoopbackAndVirtual(t *testing.T) {
	for _, address := range LocalAddresses() {
		if strings.HasPrefix(address.IP, "127.") {
			t.Errorf("loopback %s should not be offered", address.IP)
		}
		if strings.HasPrefix(address.IP, "169.254.") {
			t.Errorf("link-local %s should not be offered", address.IP)
		}
		if address.Label == "" {
			t.Errorf("%s has no label to show somebody choosing between addresses", address.IP)
		}
	}
}

func TestAdvertiseIPsIsCommaSeparated(t *testing.T) {
	got := AdvertiseIPs()
	if got == "" {
		t.Skip("no non-virtual addresses on this machine")
	}
	for _, ip := range strings.Split(got, ",") {
		if ip == "" {
			t.Fatalf("AdvertiseIPs produced an empty entry: %q", got)
		}
	}
}

func TestSharedComposeAdvertisesTheMachinesAddresses(t *testing.T) {
	store := NewStore(t.TempDir())
	path, err := store.WriteSharedCompose()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := readFile(path)
	if !strings.Contains(body, "ICE_ADVERTISE_IP") {
		t.Fatalf("the shared SFU does not advertise anything:\n%s", body)
	}
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	return string(data), err
}
