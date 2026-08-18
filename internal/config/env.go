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
		{Key: "STORAGE_BACKEND", Value: p.StorageBackend, Mode: ModeRestart},
		// The server refuses to start without this, deliberately: it treats
		// the placeholder as fatal rather than signing tokens with a value
		// everybody knows.
		{Key: "JWT_SECRET", Value: p.JWTSecret, Sensitive: true, Mode: ModeRestart},
	}
	if p.SFUWebSocketURL != "" {
		settings = append(settings, EnvSetting{Key: "SFU_WS_HOST", Value: p.SFUWebSocketURL, Mode: ModeRestart})
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

func (s *Store) WriteEnv(profile Profile) (string, error) {
	if err := profile.Validate(); err != nil {
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
	for _, setting := range profile.EnvSettings() {
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
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "node", "-e", "fetch('http://127.0.0.1:%d/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"]
      interval: 30s
      timeout: 10s
      retries: 3
`, profile.ID, profile.Host, profile.Port, profile.Port, profile.Port)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
