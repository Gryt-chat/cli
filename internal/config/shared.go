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
	// MinIOContainer is the object store every server here shares. One store
	// with one bucket, because the alternative is a MinIO per server on a
	// machine that already runs one process per server.
	MinIOContainer = "gryt-minio"
	// MinIOPort is the port inside the network. It is deliberately not
	// published to the host: servers reach the store over the shared network
	// by name, and publishing it only creates a collision. On this machine it
	// collided immediately with an unrelated MinIO already on 9000, and the
	// failure surfaced as "failed to set up container networking", which says
	// nothing about ports.
	MinIOPort = 9000
	// SFUPort is the signalling port inside the network.
	SFUPort = 5005
	// DefaultSTUN matches what production runs. Without it the server logs
	// "Missing STUN servers! SFU may not reach all clients" and media fails
	// for anybody behind NAT, which is most people. STUN reveals a client's
	// address to the STUN server and nothing else; it is set here rather than
	// left empty because voice that only works on one LAN is not voice.
	DefaultSTUN = "stun:stun.l.google.com:19302,stun:stun1.l.google.com:19302"
	// DefaultSTUNServer is the same first server, in the host:port form a
	// binding request needs. Named once so the docs can say which host the
	// public-address lookup contacts.
	DefaultSTUNServer = "stun.l.google.com:19302"
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

// InternalS3Endpoint is how a server reaches the shared object store.
func InternalS3Endpoint() string {
	return "http://" + MinIOContainer + ":" + strconv.Itoa(MinIOPort)
}

func (s *Store) WriteSharedCompose() (string, error) {
	dir := s.SharedDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	secrets, err := s.Secrets()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "compose.yaml")
	content := `# Managed by gryt. One of each, shared by every server on this machine.
name: gryt-shared

services:
  sfu:
    image: ghcr.io/gryt-chat/sfu:` + s.Preferences().ImageTag() + `
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
      # Every address this machine answers on, so a client can find a path to
      # it. Derived from the interfaces at write time rather than asked about,
      # because it describes the machine and not any one server.
      ICE_ADVERTISE_IP: "` + AdvertiseIPs() + `"
    networks:
      - ` + SharedNetwork + `
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:5005/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s

  minio:
    image: minio/minio:latest
    container_name: ` + MinIOContainer + `
    command: ["server", "/data", "--console-address", ":9001"]
    environment:
      MINIO_ROOT_USER: "` + secrets.MinIOUser + `"
      MINIO_ROOT_PASSWORD: "` + secrets.MinIOPassword + `"
    volumes:
      - minio-data:/data
    networks:
      - ` + SharedNetwork + `
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "mc", "ready", "local"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 10s

  # Creates the bucket once. Exits, and compose leaves it exited.
  minio-init:
    image: minio/mc:latest
    container_name: ` + MinIOContainer + `-init
    depends_on:
      minio:
        condition: service_healthy
    environment:
      MINIO_ROOT_USER: "` + secrets.MinIOUser + `"
      MINIO_ROOT_PASSWORD: "` + secrets.MinIOPassword + `"
      S3_BUCKET: "` + secrets.Bucket + `"
    entrypoint: ["/bin/sh", "-c"]
    command:
      - |
        set -e
        mc alias set local ` + InternalS3Endpoint() + ` "$$MINIO_ROOT_USER" "$$MINIO_ROOT_PASSWORD"
        mc mb -p "local/$$S3_BUCKET" 2>/dev/null || true
        echo "Bucket ready: $$S3_BUCKET"
    networks:
      - ` + SharedNetwork + `
    restart: "no"

volumes:
  minio-data:

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
