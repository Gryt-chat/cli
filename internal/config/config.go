package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type SecurityLevel string

const (
	SecurityStrict    SecurityLevel = "strict"
	SecurityBalanced  SecurityLevel = "balanced"
	SecurityCommunity SecurityLevel = "community"
)

var securityLevels = []SecurityLevel{SecurityStrict, SecurityBalanced, SecurityCommunity}

func SecurityLevels() []SecurityLevel {
	return append([]SecurityLevel(nil), securityLevels...)
}

func (s SecurityLevel) Description() string {
	switch s {
	case SecurityStrict:
		return "Accounts only, hidden from discovery, invite-only"
	case SecurityCommunity:
		return "Accounts and local identities, discoverable, invite-only"
	default:
		return "Accounts only, discoverable, invite-only"
	}
}

type Profile struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Host             string        `json:"host"`
	Port             int           `json:"port"`
	Security         SecurityLevel `json:"security"`
	DataDir          string        `json:"dataDir"`
	SFUWebSocketURL  string        `json:"sfuWebSocketUrl,omitempty"`
	VoiceMaxUsers    int           `json:"voiceMaxUsers"`
	TrustedProxyHops int           `json:"trustedProxyHops"`
	StorageBackend   string        `json:"storageBackend"`
	// Signs this server's session tokens. Generated once and kept, because
	// rotating it signs everybody out.
	JWTSecret string `json:"jwtSecret,omitempty"`
	// Authorises the CLI against this server's management API. Kept here
	// rather than in the generated .env on purpose: .env is the file somebody
	// pastes into a bug report or copies to another machine, and this is a
	// credential for a running server. It reaches the container through the
	// compose command's own environment instead.
	AdminToken string `json:"adminToken,omitempty"`
	// The management API's port on this machine. Published to loopback only,
	// and its own port because the server's main one is reachable by design.
	AdminPort int `json:"adminPort,omitempty"`
	// Filled in when the server uses this machine's shared object store, so
	// the generated files can name its credentials. Deliberately not
	// persisted: the shared secrets file owns them, and copying them into
	// every profile would mean rotating them in several places.
	SharedS3  *SharedSecrets    `json:"-"`
	ExtraEnv  map[string]string `json:"extraEnv,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

func NewProfile(name string) Profile {
	now := time.Now().UTC()
	return Profile{
		ID:             Slug(name),
		Name:           strings.TrimSpace(name),
		Host:           "0.0.0.0",
		Port:           5000,
		Security:       SecurityBalanced,
		DataDir:        "/data",
		VoiceMaxUsers:  0,
		StorageBackend: SharedStorage,
		JWTSecret:      NewSecret(),
		AdminToken:     NewSecret(),
		ExtraEnv:       map[string]string{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// NewSecret returns a secret suitable for signing session tokens. 48 random
// bytes, which is what the deployment docs tell people to generate by hand.
func NewSecret() string {
	buf := make([]byte, 48)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand does not fail on any platform this runs on, and a
		// predictable fallback would be worse than not starting.
		panic("gryt: no entropy available: " + err.Error())
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func (p Profile) Address() string {
	return net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
}

func (p Profile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("server name is required")
	}
	if p.ID == "" || p.ID != Slug(p.ID) {
		return errors.New("server id must contain letters, numbers, or hyphens")
	}
	if net.ParseIP(p.Host) == nil && p.Host != "localhost" {
		return fmt.Errorf("host %q is not a valid IP address or localhost", p.Host)
	}
	if p.JWTSecret == "" {
		return errors.New("server has no JWT secret")
	}
	if p.Port < 1 || p.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	validSecurity := false
	for _, level := range securityLevels {
		validSecurity = validSecurity || p.Security == level
	}
	if !validSecurity {
		return fmt.Errorf("unknown security level %q", p.Security)
	}
	if p.VoiceMaxUsers < 0 || p.VoiceMaxUsers > 100000 {
		return errors.New("voice seats must be between 0 and 100000")
	}
	if p.TrustedProxyHops < 0 || p.TrustedProxyHops > 16 {
		return errors.New("trusted proxy hops must be between 0 and 16")
	}
	// "shared" is this machine's own object store. The server never sees that
	// word: EnvSettings maps it to s3 and points it at the store next door.
	if p.StorageBackend != "filesystem" && p.StorageBackend != "s3" && p.StorageBackend != SharedStorage {
		return errors.New("storage backend must be shared, filesystem or s3")
	}
	return nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func Slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonSlug.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

type Store struct {
	root string
}

func DefaultRoot() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("GRYT_CONFIG_DIR")); custom != "" {
		return custom, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "gryt"), nil
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

func (s *Store) Root() string { return s.root }

func (s *Store) profilePath(id string) string {
	return filepath.Join(s.root, "servers", Slug(id), "profile.json")
}

func (s *Store) ServerDir(id string) string {
	return filepath.Dir(s.profilePath(id))
}

func (s *Store) List() ([]Profile, error) {
	pattern := filepath.Join(s.root, "servers", "*", "profile.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(paths))
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", path, readErr)
		}
		var profile Profile
		if jsonErr := json.Unmarshal(data, &profile); jsonErr != nil {
			return nil, fmt.Errorf("decode %s: %w", path, jsonErr)
		}
		// Profiles written before the CLI generated a secret have none, and a
		// server without one refuses to start. Filling it in here, on the read
		// path, is what makes an existing profile work on the next start
		// rather than requiring the operator to recreate it. Written back so
		// the value is stable: generating a fresh one on every load would sign
		// everybody out each time.
		if profile.JWTSecret == "" {
			profile.JWTSecret = NewSecret()
			// Best effort. If the profile cannot be written back, for any
			// reason including it being invalid in some unrelated way, the
			// listing still succeeds and this server still gets a working
			// secret for as long as the process lives. Failing the whole
			// listing because one profile could not be migrated would take
			// every other server down with it.
			_ = s.Save(profile)
		}
		if profile.AdminToken == "" {
			profile.AdminToken = NewSecret()
			_ = s.Save(profile)
		}
		profiles = append(profiles, profile)
	}
	// A management port cannot be chosen while the profiles are still being
	// read, because picking one needs to know what every other server already
	// claims. Second pass, once they are all here.
	for i := range profiles {
		if profiles[i].AdminPort != 0 {
			continue
		}
		profiles[i].AdminPort = FreeAdminPort(PortsInUse(profiles))
		_ = s.Save(profiles[i])
	}

	sort.Slice(profiles, func(i, j int) bool {
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})
	return profiles, nil
}

func (s *Store) Save(profile Profile) error {
	if profile.ID == "" {
		profile.ID = Slug(profile.Name)
	}
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = time.Now().UTC()
	}
	profile.UpdatedAt = time.Now().UTC()
	if err := profile.Validate(); err != nil {
		return err
	}
	dir := s.ServerDir(profile.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create server directory: %w", err)
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(s.profilePath(profile.ID), data, 0o600); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}
	return nil
}
