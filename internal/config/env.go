package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type SettingMode string

const (
	ModeLive    SettingMode = "live"
	ModeRestart SettingMode = "restart"
)

type EnvSetting struct {
	Key       string
	Value     string
	Sensitive bool
	Mode      SettingMode
}

func (p Profile) EnvSettings() []EnvSetting {
	authMode := "required"
	identityTiers := "account"
	discoverable := "true"
	if p.Security == SecurityStrict {
		discoverable = "false"
	}
	if p.Security == SecurityCommunity {
		identityTiers = "account,local"
	}

	settings := []EnvSetting{
		{Key: "SERVER_NAME", Value: p.Name, Mode: ModeLive},
		// The generated deployment is a container. It must listen on the
		// container interface; Profile.Host controls the host-side port binding.
		{Key: "HOST", Value: "0.0.0.0", Mode: ModeRestart},
		{Key: "PORT", Value: strconv.Itoa(p.Port), Mode: ModeRestart},
		{Key: "DATA_DIR", Value: p.DataDir, Mode: ModeRestart},
		{Key: "GRYT_AUTH_MODE", Value: authMode, Mode: ModeRestart},
		{Key: "GRYT_IDENTITY_TIERS", Value: identityTiers, Mode: ModeRestart},
		{Key: "SERVER_DISCOVERABLE", Value: discoverable, Mode: ModeLive},
		{Key: "GRYT_TRUSTED_PROXY_HOPS", Value: strconv.Itoa(p.TrustedProxyHops), Mode: ModeRestart},
		{Key: "VOICE_MAX_USERS", Value: strconv.Itoa(p.VoiceMaxUsers), Mode: ModeRestart},
		{Key: "STORAGE_BACKEND", Value: storageBackend(p.StorageBackend), Mode: ModeRestart},
		// The server refuses to start without this, deliberately: it treats
		// the placeholder as fatal rather than signing tokens with a value
		// everybody knows.
		{Key: "JWT_SECRET", Value: p.JWTSecret, Sensitive: true, Mode: ModeRestart},
	}
	// The server reaches the SFU over the shared network by container name.
	// This is not the address clients dial: that is SFU_PUBLIC_HOST below,
	// which depends on how this machine is reachable from wherever they are.
	settings = append(settings, EnvSetting{Key: "SFU_WS_HOST", Value: InternalSFUHost(), Mode: ModeRestart})

	// What a client is told to connect to. Falls back to localhost, which is
	// right for trying a server on the machine that hosts it and wrong for
	// anything else, so the wizard asks.
	public := p.SFUWebSocketURL
	if public == "" {
		public = "ws://localhost:" + strconv.Itoa(SFUPort)
	}
	settings = append(settings, EnvSetting{Key: "SFU_PUBLIC_HOST", Value: public, Mode: ModeRestart})

	// The server hands these to clients for ICE. Without them it logs
	// "Missing STUN servers!" and media fails for anybody behind NAT.
	stun := DefaultSTUN
	if override, ok := p.ExtraEnv["STUN_SERVERS"]; ok {
		stun = override
	}
	settings = append(settings, EnvSetting{Key: "STUN_SERVERS", Value: stun, Mode: ModeRestart})

	// Pointed at the object store beside it, with credentials generated once
	// for this machine rather than the published minioadmin defaults.
	if p.StorageBackend == SharedStorage && p.SharedS3 != nil {
		settings = append(settings,
			EnvSetting{Key: "S3_ENDPOINT", Value: InternalS3Endpoint(), Mode: ModeRestart},
			EnvSetting{Key: "S3_REGION", Value: "auto", Mode: ModeRestart},
			EnvSetting{Key: "S3_ACCESS_KEY_ID", Value: p.SharedS3.MinIOUser, Sensitive: true, Mode: ModeRestart},
			EnvSetting{Key: "S3_SECRET_ACCESS_KEY", Value: p.SharedS3.MinIOPassword, Sensitive: true, Mode: ModeRestart},
			EnvSetting{Key: "S3_BUCKET", Value: p.SharedS3.Bucket, Mode: ModeRestart},
			EnvSetting{Key: "S3_FORCE_PATH_STYLE", Value: "true", Mode: ModeRestart},
			// The image worker runs beside this server rather than in the
			// shared project: it reads the job queue out of this server's
			// SQLite database, so it needs this server's data directory and
			// cannot be one process for all of them.
			EnvSetting{Key: "IMAGE_WORKER_URL", Value: "http://gryt-" + p.ID + "-image-worker:8080", Mode: ModeRestart},
		)
	}
	keys := make([]string, 0, len(p.ExtraEnv))
	for key := range p.ExtraEnv {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		settings = append(settings, EnvSetting{
			Key:       key,
			Value:     p.ExtraEnv[key],
			Sensitive: isSensitiveKey(key),
			Mode:      ModeRestart,
		})
	}
	return settings
}

// storageBackend maps the wizard's answer onto what the server understands.
// "shared" is a deployment arrangement rather than a backend: to the server it
// is S3, pointed at the object store running beside it.
func storageBackend(choice string) string {
	if choice == SharedStorage {
		return "s3"
	}
	return choice
}

// SharedStorage is the wizard's name for using this machine's own object store.
const SharedStorage = "shared"

func isSensitiveKey(key string) bool {
	upper := strings.ToUpper(key)
	return strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "ACCESS_KEY")
}

func quoteEnv(value string) string {
	if value == "" {
		return `""`
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return r == ' ' || r == '#' || r == '"' || r == '\'' || r == '\\' || r == '\n' || r == '\r'
	}) == -1 {
		return value
	}
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r")
	return `"` + replacer.Replace(value) + `"`
}

// Settings resolves what this server's environment actually is.
//
// EnvSettings alone is not enough for a server on the shared object store: its
// credentials live in the shared secrets file rather than on the profile, so
// they have to be attached first. Doing that here rather than at each call site
// is what stopped `gryt env` from reporting STORAGE_BACKEND=s3 with no S3
// settings under it while the generated .env had all of them.
func (s *Store) Settings(profile Profile) ([]EnvSetting, error) {
	if profile.StorageBackend == SharedStorage {
		secrets, err := s.Secrets()
		if err != nil {
			return nil, err
		}
		profile.SharedS3 = &secrets
	}
	return profile.EnvSettings(), nil
}

func (s *Store) WriteEnv(profile Profile) (string, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}
	settings, err := s.Settings(profile)
	if err != nil {
		return "", err
	}
	dir := s.ServerDir(profile.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, ".env")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	w := bufio.NewWriter(file)
	_, _ = fmt.Fprintln(w, "# Managed by gryt. Edit through the TUI to preserve validation.")
	for _, setting := range settings {
		_, _ = fmt.Fprintf(w, "%s=%s\n", setting.Key, quoteEnv(setting.Value))
	}
	if err := w.Flush(); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) WriteCompose(profile Profile) (string, error) {
	dir := s.ServerDir(profile.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "compose.yaml")

	// The image worker reads the job queue out of this server's SQLite
	// database, so it mounts this server's data directory and there is one per
	// server. It cannot live in the shared project with the SFU and the object
	// store, which serve every server from one process each.
	worker := ""
	if profile.StorageBackend == SharedStorage {
		secrets, err := s.Secrets()
		if err != nil {
			return "", err
		}
		worker = fmt.Sprintf(`
  image-worker:
    image: ghcr.io/gryt-chat/image-worker:latest
    container_name: gryt-%s-image-worker
    environment:
      DATA_DIR: /data
      S3_ENDPOINT: %s
      S3_REGION: auto
      S3_ACCESS_KEY_ID: "%s"
      S3_SECRET_ACCESS_KEY: "%s"
      S3_BUCKET: "%s"
      S3_FORCE_PATH_STYLE: "true"
    volumes:
      - ./data:/data
    depends_on:
      server:
        condition: service_healthy
    networks:
      - %s
    restart: unless-stopped
`, profile.ID, InternalS3Endpoint(), secrets.MinIOUser, secrets.MinIOPassword, secrets.Bucket, SharedNetwork)
	}

	content := fmt.Sprintf(`services:
  server:
    image: ghcr.io/gryt-chat/server:latest
    container_name: gryt-%s
    env_file:
      - .env
    ports:
      - "%s:%d:%d"
    volumes:
      - ./data:/data
    networks:
      - `+SharedNetwork+`
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "node", "-e", "fetch('http://127.0.0.1:%d/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"]
      interval: 30s
      timeout: 10s
      retries: 3

%s
# Created by the shared project, which holds the SFU and the object store.
networks:
  `+SharedNetwork+`:
    external: true
`, profile.ID, profile.Host, profile.Port, profile.Port, profile.Port, worker)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
