package create

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gryt-chat/cli/internal/config"
)

func run(t *testing.T, store *config.Store, opts Options) (config.Profile, string, error) {
	t.Helper()
	var out strings.Builder
	profile, err := Run(context.Background(), &out, store, opts)
	return profile, out.String(), err
}

func TestWithoutYesPreviewsAndWritesNothing(t *testing.T) {
	store := config.NewStore(t.TempDir())

	profile, out, err := run(t, store, Options{Name: "My Server"})
	if !errors.Is(err, ErrPreview) {
		t.Fatalf("expected ErrPreview, got %v", err)
	}
	if profile.ID != "my-server" {
		t.Fatalf("expected the profile it would create back, got %+v", profile)
	}
	if !strings.Contains(out, "Nothing created") {
		t.Fatalf("expected a preview notice, got:\n%s", out)
	}
	if profiles, _ := store.List(); len(profiles) != 0 {
		t.Fatalf("a preview wrote a profile: %v", profiles)
	}
}

func TestYesWritesTheProfileEnvAndCompose(t *testing.T) {
	store := config.NewStore(t.TempDir())

	profile, out, err := run(t, store, Options{Name: "My Server", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Created My Server") {
		t.Fatalf("expected a created notice, got:\n%s", out)
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ID != profile.ID {
		t.Fatalf("expected the new profile listed, got %v", profiles)
	}
	dir := store.ServerDir(profile.ID)
	for _, name := range []string{"profile.json", ".env", "compose.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected %s written: %v", name, err)
		}
	}
}

func TestDefaultsMatchTheWizard(t *testing.T) {
	store := config.NewStore(t.TempDir())

	profile, _, err := run(t, store, Options{Name: "My Server", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Host != "0.0.0.0" {
		t.Fatalf("expected the wizard's default bind address, got %q", profile.Host)
	}
	if profile.Security != config.SecurityBalanced {
		t.Fatalf("expected the wizard's recommended security level, got %q", profile.Security)
	}
	if profile.VoiceMaxUsers != 0 || profile.TrustedProxyHops != 0 {
		t.Fatalf("expected no cap and no proxy hops, got %+v", profile)
	}
	if profile.SFUWebSocketURL != "ws://localhost:5005" {
		t.Fatalf("expected only localhost reachable by default, got %q", profile.SFUWebSocketURL)
	}
}

func TestFlagsOverrideEveryDefault(t *testing.T) {
	store := config.NewStore(t.TempDir())

	profile, _, err := run(t, store, Options{
		Name: "Edge", Host: "127.0.0.1", Port: 6100, Security: "strict",
		VoiceSeats: 12, ProxyHops: 1, Domains: []string{"wss://voice.example.com"}, Yes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Host != "127.0.0.1" || profile.Port != 6100 || profile.Security != config.SecurityStrict {
		t.Fatalf("flags were not applied: %+v", profile)
	}
	if profile.VoiceMaxUsers != 12 || profile.TrustedProxyHops != 1 {
		t.Fatalf("flags were not applied: %+v", profile)
	}
	if !strings.Contains(profile.SFUWebSocketURL, "wss://voice.example.com") {
		t.Fatalf("the typed domain is missing from %q", profile.SFUWebSocketURL)
	}
}

func TestLANAddsThisMachinesAddresses(t *testing.T) {
	store := config.NewStore(t.TempDir())

	profile, _, err := run(t, store, Options{Name: "Lan", LAN: true, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	want := len(config.LocalAddresses()) + 1 // localhost plus every LAN address
	got := len(strings.Split(profile.SFUWebSocketURL, ","))
	if got != want {
		t.Fatalf("expected %d reachable addresses with --lan, got %d (%q)", want, got, profile.SFUWebSocketURL)
	}
}

func TestAPortOutsideRangeIsRejected(t *testing.T) {
	store := config.NewStore(t.TempDir())

	if _, _, err := run(t, store, Options{Name: "Bad Port", Port: 70000, Yes: true}); err == nil {
		t.Fatal("expected a validation error")
	}
}

func TestABadDomainIsRejected(t *testing.T) {
	store := config.NewStore(t.TempDir())

	_, _, err := run(t, store, Options{Name: "Bad Domain", Domains: []string{"voice.example.com"}, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "ws://") {
		t.Fatalf("expected a scheme complaint, got %v", err)
	}
}

func TestCreatingTheSameNameTwiceIsRefused(t *testing.T) {
	store := config.NewStore(t.TempDir())
	if _, _, err := run(t, store, Options{Name: "Dup", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, store, Options{Name: "Dup", Yes: true}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected an already-exists refusal, got %v", err)
	}
}

func TestSecondServerGetsAFreePort(t *testing.T) {
	store := config.NewStore(t.TempDir())
	first, _, err := run(t, store, Options{Name: "First", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := run(t, store, Options{Name: "Second", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if second.Port == first.Port || second.AdminPort == first.AdminPort {
		t.Fatalf("expected distinct ports, got %+v and %+v", first, second)
	}
}
