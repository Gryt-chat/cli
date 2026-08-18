package config

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// stubSTUN answers one binding request with an XOR-MAPPED-ADDRESS of the given
// family, so the parser can be exercised without the internet and without
// depending on which family a real server happens to answer over.
func stubSTUN(t *testing.T, family byte, ip net.IP) string {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	go func() {
		buf := make([]byte, 1024)
		n, addr, err := conn.ReadFrom(buf)
		if err != nil || n < 20 {
			return
		}
		transaction := buf[8:20]

		value := []byte{0x00, family, 0x00, 0x00}
		const magic = 0x2112A442
		if family == 0x01 {
			encoded := make([]byte, 4)
			binary.BigEndian.PutUint32(encoded, binary.BigEndian.Uint32(ip.To4())^magic)
			value = append(value, encoded...)
		} else {
			key := make([]byte, 0, 16)
			cookie := make([]byte, 4)
			binary.BigEndian.PutUint32(cookie, magic)
			key = append(key, cookie...)
			key = append(key, transaction...)
			raw := ip.To16()
			for i := 0; i < 16; i++ {
				value = append(value, raw[i]^key[i])
			}
		}

		reply := make([]byte, 20)
		binary.BigEndian.PutUint16(reply[0:2], 0x0101)
		binary.BigEndian.PutUint16(reply[2:4], uint16(4+len(value)))
		binary.BigEndian.PutUint32(reply[4:8], magic)
		copy(reply[8:20], transaction)

		attr := make([]byte, 4)
		binary.BigEndian.PutUint16(attr[0:2], 0x0020)
		binary.BigEndian.PutUint16(attr[2:4], uint16(len(value)))

		_, _ = conn.WriteTo(append(reply, append(attr, value...)...), addr)
	}()

	return conn.LocalAddr().String()
}

func TestStunReadsAnIPv4MappedAddress(t *testing.T) {
	server := stubSTUN(t, 0x01, net.ParseIP("203.0.113.7"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := stunBinding(ctx, "udp4", server)
	if err != nil {
		t.Fatal(err)
	}
	if got != "203.0.113.7" {
		t.Fatalf("got %q", got)
	}
}

// The case that broke it against the real server: a dual-stack machine reaches
// Google's STUN over IPv6 and is answered with family 0x02, which the first
// version walked straight past.
func TestStunReadsAnIPv6MappedAddress(t *testing.T) {
	server := stubSTUN(t, 0x02, net.ParseIP("2001:db8::1"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := stunBinding(ctx, "udp4", server)
	if err != nil {
		t.Fatal(err)
	}
	if net.ParseIP(got) == nil || !net.ParseIP(got).Equal(net.ParseIP("2001:db8::1")) {
		t.Fatalf("got %q", got)
	}
}

func TestStunGivesUpRatherThanHanging(t *testing.T) {
	// Nothing is listening, so the read must time out rather than block.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := stunBinding(ctx, "udp4", "127.0.0.1:9"); err == nil {
		t.Fatal("a silent server should not look like a successful lookup")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("the lookup outlived its context")
	}
}

func TestTheLookupCanBeTurnedOff(t *testing.T) {
	t.Setenv("GRYT_NO_PUBLIC_LOOKUP", "1")
	if !PublicLookupDisabled() {
		t.Fatal("GRYT_NO_PUBLIC_LOOKUP was ignored")
	}
	// And nothing is sent when it is set.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := PublicAddress(ctx, "127.0.0.1:9"); err == nil {
		t.Fatal("a disabled lookup should report that rather than succeeding")
	}
}
