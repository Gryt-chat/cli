package config

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"strings"
	"time"
)

// PublicLookupDisabled reports whether the operator has asked not to be told
// their public address. The lookup leaves the machine, so it gets an off switch
// and the docs name the host it contacts.
func PublicLookupDisabled() bool {
	return strings.TrimSpace(os.Getenv("GRYT_NO_PUBLIC_LOOKUP")) != ""
}

// PublicAddress reports the address this machine is reachable at from outside
// its own network.
//
// Interfaces cannot answer this. A machine behind NAT holds a private address
// and has no way of knowing what the world sees, so something outside has to
// say. STUN exists for precisely that question, the SFU already depends on it
// for voice, and the default server is the one already named in the shared
// stack's configuration — so this adds no new dependency and no new party.
//
// A machine that genuinely holds a public address on an interface is answered
// from the interface, and nothing is sent at all.
func PublicAddress(ctx context.Context, server string) (string, error) {
	for _, address := range LocalAddresses() {
		if ip := net.ParseIP(address.IP); ip != nil && !ip.IsPrivate() {
			return address.IP, nil
		}
	}
	if PublicLookupDisabled() {
		return "", errors.New("public address lookup is turned off")
	}
	// IPv4 first. A dual-stack machine reaches Google's STUN over IPv6 and is
	// told its IPv6 address, which is correct and almost never what somebody
	// wants: port forwarding, and most of what people hand out, is v4. Fall
	// back to whatever the network offers when there is no v4 path at all.
	if address, err := stunBinding(ctx, "udp4", server); err == nil {
		return address, nil
	}
	return stunBinding(ctx, "udp", server)
}

// stunBinding sends one STUN binding request and reads the mapped address out
// of the reply. RFC 5389 in about forty lines: a fixed header, a random
// transaction id, and one attribute worth reading.
func stunBinding(ctx context.Context, network, server string) (string, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(3 * time.Second)
	}

	conn, err := net.Dial(network, server)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		return "", err
	}

	const magic = 0x2112A442
	request := make([]byte, 20)
	binary.BigEndian.PutUint16(request[0:2], 0x0001) // binding request
	binary.BigEndian.PutUint16(request[2:4], 0)      // no attributes
	binary.BigEndian.PutUint32(request[4:8], magic)
	if _, err := rand.Read(request[8:20]); err != nil {
		return "", err
	}

	if _, err := conn.Write(request); err != nil {
		return "", err
	}

	reply := make([]byte, 1024)
	n, err := conn.Read(reply)
	if err != nil {
		return "", err
	}
	if n < 20 || binary.BigEndian.Uint16(reply[0:2]) != 0x0101 {
		return "", errors.New("not a STUN binding response")
	}
	// Same transaction, or it is somebody else's reply.
	if string(reply[8:20]) != string(request[8:20]) {
		return "", errors.New("STUN response does not match the request")
	}

	body := reply[20:n]
	for len(body) >= 4 {
		kind := binary.BigEndian.Uint16(body[0:2])
		length := int(binary.BigEndian.Uint16(body[2:4]))
		if 4+length > len(body) {
			return "", errors.New("truncated STUN attribute")
		}
		value := body[4 : 4+length]

		// XOR-MAPPED-ADDRESS. The address is obfuscated with the magic cookie
		// so that NATs rewriting payloads cannot accidentally mangle it.
		if kind == 0x0020 && len(value) >= 8 {
			switch value[1] {
			case 0x01: // IPv4: four bytes against the cookie
				ip := make(net.IP, 4)
				binary.BigEndian.PutUint32(ip, binary.BigEndian.Uint32(value[4:8])^magic)
				return ip.String(), nil
			case 0x02: // IPv6: sixteen bytes against the cookie and the transaction id
				if len(value) < 20 {
					return "", errors.New("truncated IPv6 mapped address")
				}
				key := append(request[4:8:8], request[8:20]...)
				ip := make(net.IP, 16)
				for i := 0; i < 16; i++ {
					ip[i] = value[4+i] ^ key[i]
				}
				return ip.String(), nil
			}
		}

		// Attributes are padded to a four-byte boundary.
		advance := 4 + length
		if pad := advance % 4; pad != 0 {
			advance += 4 - pad
		}
		body = body[advance:]
	}
	return "", errors.New("STUN response carried no mapped address")
}
