package config

import (
	"os"
	"path/filepath"
	"strconv"
)

// The shared stack: the pieces every server on this machine uses one of.
//
// The SFU is built for this. One process serves every Gryt server on a host,
// which is how production has always run it, so giving each server its own
// would be both wasteful and unlike the deployment everything else is tested
// against.
//
// It is a separate compose project rather than another service in each
// server's file, so that starting and stopping a server stays a per-server
// operation and does not take the media plane down with it.
const (
	// SharedNetwork is created by the shared project and joined by each server
	// as external. The name is fixed because the server files have to name it
	// without reading anything else.
	SharedNetwork = "gryt"
	// SFUContainer is how servers address the SFU. A container name rather
	// than a service name, so it resolves the same way from another compose
	// project on the shared network.
	SFUContainer = "gryt-sfu"
	// SFUPort is the signalling port inside the network.
	SFUPort = 5005
	// DefaultSTUN matches what production runs. Without it the server logs
	// "Missing STUN servers! SFU may not reach all clients" and media fails
	// for anybody behind NAT, which is most people. STUN reveals a client's
	// address to the STUN server and nothing else; it is set here rather than
	// left empty because voice that only works on one LAN is not voice.
	DefaultSTUN = "stun:stun.l.google.com:19302,stun:stun1.l.google.com:19302"
	// SFUMuxPort carries every participant's media over one UDP port.
	// Production uses 443, which needs a privileged bind and collides with
	// anything already serving HTTPS; 3478 is unprivileged and is the port
	// people already open for STUN.
	SFUMuxPort = 3478
)

// SharedDir holds the shared project, beside the servers rather than inside
// any one of them.
func (s *Store) SharedDir() string {
	return filepath.Join(s.root, "shared")
}

// InternalSFUHost is what a server uses to reach the SFU over the shared
// network. Not what a client uses: that is SFU_PUBLIC_HOST, which depends on
// how this machine is reachable and is asked for separately.
func InternalSFUHost() string {
	return "ws://" + SFUContainer + ":5005"
}

func (s *Store) WriteSharedCompose() (string, error) {
	dir := s.SharedDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "compose.yaml")
	content := `# Managed by gryt. One of each, shared by every server on this machine.
name: gryt-shared

services:
  sfu:
    image: ghcr.io/gryt-chat/sfu:latest
    container_name: ` + SFUContainer + `
    ports:
      - "` + strconv.Itoa(SFUPort) + `:5005"
      - "` + strconv.Itoa(SFUMuxPort) + `:` + strconv.Itoa(SFUMuxPort) + `/udp"
    environment:
      PORT: "5005"
      # One UDP port for every participant, so there is no port range to size
      # and nothing to run out of.
      ICE_UDP_MUX_PORT: "` + strconv.Itoa(SFUMuxPort) + `"
      STUN_SERVERS: "` + DefaultSTUN + `"
    networks:
      - ` + SharedNetwork + `
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:5005/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s

networks:
  ` + SharedNetwork + `:
    name: ` + SharedNetwork + `
    driver: bridge
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
