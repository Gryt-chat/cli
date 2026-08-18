package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedComposeCarriesTheSFUAndCreatesTheNetwork(t *testing.T) {
	store := NewStore(t.TempDir())
	path, err := store.WriteSharedCompose()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	yaml := string(body)

	for _, want := range []string{
		"ghcr.io/gryt-chat/sfu",
		"container_name: " + SFUContainer,
		"ICE_UDP_MUX_PORT",
		"name: " + SharedNetwork,
	} {
		if !strings.Contains(yaml, want) {
			t.Fatalf("shared compose is missing %q:\n%s", want, yaml)
		}
	}
	// The shared project owns the network; it must not declare it external or
	// nothing would ever create it.
	if strings.Contains(yaml, "external: true") {
		t.Fatal("the shared project must create the network, not borrow it")
	}
}

func TestServerComposeJoinsTheSharedNetworkWithoutCreatingIt(t *testing.T) {
	store := NewStore(t.TempDir())
	profile := NewProfile("My Server")
	path, err := store.WriteCompose(profile)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	yaml := string(body)

	if !strings.Contains(yaml, "external: true") {
		t.Fatalf("a server must borrow the shared network, not define a second one:\n%s", yaml)
	}
	if !strings.Contains(yaml, SharedNetwork) {
		t.Fatalf("server compose does not join %s:\n%s", SharedNetwork, yaml)
	}
}

// The two halves of the SFU configuration answer different questions, and
// confusing them is how voice half-works: the server talks to the container,
// the client is told an address it can actually reach.
func TestSFUEnvSplitsInternalFromPublic(t *testing.T) {
	profile := NewProfile("My Server")

	settings := map[string]string{}
	for _, s := range profile.EnvSettings() {
		settings[s.Key] = s.Value
	}

	if settings["SFU_WS_HOST"] != InternalSFUHost() {
		t.Fatalf("SFU_WS_HOST = %q, want the shared container", settings["SFU_WS_HOST"])
	}
	if !strings.Contains(settings["SFU_PUBLIC_HOST"], "localhost") {
		t.Fatalf("SFU_PUBLIC_HOST = %q, want a localhost default", settings["SFU_PUBLIC_HOST"])
	}

	profile.SFUWebSocketURL = "ws://192.168.1.20:5005"
	settings = map[string]string{}
	for _, s := range profile.EnvSettings() {
		settings[s.Key] = s.Value
	}
	if settings["SFU_PUBLIC_HOST"] != "ws://192.168.1.20:5005" {
		t.Fatalf("SFU_PUBLIC_HOST = %q, want what the wizard supplied", settings["SFU_PUBLIC_HOST"])
	}
	// The internal address must not follow the public one.
	if settings["SFU_WS_HOST"] != InternalSFUHost() {
		t.Fatalf("SFU_WS_HOST = %q after setting the public address", settings["SFU_WS_HOST"])
	}
}

func TestSharedDirSitsBesideTheServers(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if store.SharedDir() != filepath.Join(root, "shared") {
		t.Fatalf("SharedDir = %q", store.SharedDir())
	}
}
