package config

import (
	"net"
	"strconv"
	"testing"
)

func TestFreePortSkipsWhatIsAlreadyListening(t *testing.T) {
	// Hold DefaultPort for the duration, standing in for whatever else on the
	// machine might have it. On macOS that is ControlCenter, for AirPlay.
	listener, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(DefaultPort)))
	if err != nil {
		t.Skipf("something already holds %d, which this test needs to control", DefaultPort)
	}
	defer listener.Close()

	if got := FreePort(nil); got == DefaultPort {
		t.Fatalf("FreePort returned %d while it was held", got)
	}
}

func TestFreePortSkipsPortsOtherServersClaim(t *testing.T) {
	got := FreePort([]int{DefaultPort, DefaultPort + 1})
	if got == DefaultPort || got == DefaultPort+1 {
		t.Fatalf("FreePort returned %d, which another server already uses", got)
	}
	if got < DefaultPort {
		t.Fatalf("FreePort returned %d, below the starting point", got)
	}
}

// The port it hands back has to be one a server can actually bind.
func TestFreePortReturnsABindablePort(t *testing.T) {
	port := FreePort(nil)
	listener, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("FreePort returned %d, which cannot be bound: %v", port, err)
	}
	_ = listener.Close()
}

func TestPortsInUseReadsTheProfiles(t *testing.T) {
	first := NewProfile("First")
	second := NewProfile("Second")
	second.Port = 5007

	ports := PortsInUse([]Profile{first, second})
	if len(ports) != 2 || ports[1] != 5007 {
		t.Fatalf("PortsInUse = %v", ports)
	}
}
