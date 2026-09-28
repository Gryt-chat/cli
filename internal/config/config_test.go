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

func writeCompose(t *testing.T, profile Profile) (*Store, string) {
	t.Helper()
	store := NewStore(t.TempDir())
	if err := store.Save(profile); err != nil {
		t.Fatal(err)
	}
	path, err := store.WriteCompose(profile)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	return store, string(data)
}

func TestComposeHandsTheDataFolderToTheServerUser(t *testing.T) {
	_, compose := writeCompose(t, NewProfile("Owned"))
	for _, want := range []string{"chown -R 1001:1001 /data", "condition: service_completed_successfully", `user: "0:0"`} {
		if !strings.Contains(compose, want) {
			t.Fatalf("compose.yaml lacks %q:\n%s", want, compose)
		}
	}
}

// The nightly timer runs compose without the CLI's environment, so the token has to
// come from a file or a recreate starts the server with no management API.
func TestTheAdminTokenComesFromAFileBesideEnv(t *testing.T) {
	profile := NewProfile("Managed")
	store, compose := writeCompose(t, profile)
	if strings.Contains(compose, "${GRYT_ADMIN_TOKEN") {
		t.Fatalf("compose.yaml still reads the token from the environment:\n%s", compose)
	}
	if !strings.Contains(compose, "- "+AdminEnvFile) {
		t.Fatalf("compose.yaml does not name %s:\n%s", AdminEnvFile, compose)
	}
	path := filepath.Join(store.ServerDir(profile.ID), AdminEnvFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "GRYT_ADMIN_TOKEN="+profile.AdminToken+"\n") {
		t.Fatalf("%s lacks the token:\n%s", AdminEnvFile, data)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("%s is %v, want 0600", AdminEnvFile, info.Mode().Perm())
	}
	env, _ := store.WriteEnv(profile)
	if data, _ := os.ReadFile(env); strings.Contains(string(data), profile.AdminToken) {
		t.Fatal(".env carries the admin token")
	}
}

func TestNoTokenMeansAnEmptyTokenFile(t *testing.T) {
	profile := NewProfile("Unmanaged")
	profile.AdminToken = ""
	store, _ := writeCompose(t, profile)
	data, err := os.ReadFile(filepath.Join(store.ServerDir(profile.ID), AdminEnvFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "GRYT_ADMIN_TOKEN") {
		t.Fatalf("a profile with no token wrote one:\n%s", data)
	}
}
