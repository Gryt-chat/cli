package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())
	profile := NewProfile("Night Owls")
	profile.Port = 5050
	if err := store.Save(profile); err != nil {
		t.Fatal(err)
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ID != "night-owls" || profiles[0].Port != 5050 {
		t.Fatalf("unexpected profiles: %#v", profiles)
	}
}

func TestEnvUsesSecurityPresetAndQuotesValues(t *testing.T) {
	store := NewStore(t.TempDir())
	profile := NewProfile("Private Room")
	profile.Security = SecurityStrict
	profile.ExtraEnv["JWT_SECRET"] = "has spaces # and symbols"
	path, err := store.WriteEnv(profile)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"SERVER_NAME=\"Private Room\"",
		"SERVER_DISCOVERABLE=false",
		"GRYT_IDENTITY_TIERS=account",
		"JWT_SECRET=\"has spaces # and symbols\"",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in:\n%s", want, text)
		}
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("server directory should be private: mode=%v err=%v", info.Mode(), err)
	}
}

func TestInvalidPort(t *testing.T) {
	profile := NewProfile("Broken")
	profile.Port = 70000
	if err := profile.Validate(); err == nil {
		t.Fatal("expected invalid port")
	}
}
