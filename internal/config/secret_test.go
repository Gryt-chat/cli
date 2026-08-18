package config

import (
	"os"
	"strings"
	"testing"
)

// The server treats a missing JWT_SECRET as fatal, so a profile without one
// produces a container that crash-loops on startup.
func TestNewProfilesGetAJWTSecret(t *testing.T) {
	profile := NewProfile("My Server")
	if profile.JWTSecret == "" {
		t.Fatal("a new profile has no JWT secret")
	}
	if len(profile.JWTSecret) < 40 {
		t.Fatalf("the secret looks too short: %d characters", len(profile.JWTSecret))
	}
	if NewProfile("Another").JWTSecret == profile.JWTSecret {
		t.Fatal("two servers were given the same secret")
	}
}

func TestTheSecretReachesTheEnvFileAndIsMasked(t *testing.T) {
	store := NewStore(t.TempDir())
	profile := NewProfile("My Server")

	path, err := store.WriteEnv(profile)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "JWT_SECRET="+profile.JWTSecret) {
		t.Fatalf("the secret did not reach .env:\n%s", body)
	}

	for _, setting := range profile.EnvSettings() {
		if setting.Key == "JWT_SECRET" && !setting.Sensitive {
			t.Fatal("JWT_SECRET must be marked sensitive so gryt env masks it")
		}
	}
}

// Rotating the secret signs everybody out, so it has to survive a round trip.
func TestTheSecretSurvivesSaveAndLoad(t *testing.T) {
	store := NewStore(t.TempDir())
	profile := NewProfile("My Server")
	if err := store.Save(profile); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].JWTSecret != profile.JWTSecret {
		t.Fatal("the secret changed across a save and load")
	}
}

// A profile written before the CLI generated secrets has none. It gets one on
// load and keeps it, rather than needing to be recreated.
func TestOlderProfilesAreGivenASecretOnce(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	profile := NewProfile("Old Server")
	profile.JWTSecret = ""
	if err := os.MkdirAll(store.ServerDir(profile.ID), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ServerDir(profile.ID)+"/profile.json",
		[]byte(`{"id":"old-server","name":"Old Server","host":"0.0.0.0","port":5000,"security":"balanced","dataDir":"/data","storageBackend":"filesystem"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].JWTSecret == "" {
		t.Fatal("an older profile was not given a secret")
	}

	second, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if second[0].JWTSecret != first[0].JWTSecret {
		t.Fatal("the secret was regenerated on the next load, which would sign everybody out")
	}
}

// One profile that cannot be written back must not take the listing with it.
func TestAProfileThatCannotBeMigratedStillLists(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := os.MkdirAll(store.ServerDir("broken"), 0o700); err != nil {
		t.Fatal(err)
	}
	// No security level, so Save refuses it.
	if err := os.WriteFile(store.ServerDir("broken")+"/profile.json",
		[]byte(`{"id":"broken","name":"Broken","host":"0.0.0.0","port":5000,"storageBackend":"filesystem"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	profiles, err := store.List()
	if err != nil {
		t.Fatalf("one unmigratable profile broke the whole listing: %v", err)
	}
	if len(profiles) != 1 || profiles[0].JWTSecret == "" {
		t.Fatal("the profile should still list, with a secret for this session")
	}
}

func TestAProfileWithoutASecretDoesNotValidate(t *testing.T) {
	profile := NewProfile("My Server")
	profile.JWTSecret = ""
	if err := profile.Validate(); err == nil {
		t.Fatal("a profile with no secret should not validate")
	}
}
