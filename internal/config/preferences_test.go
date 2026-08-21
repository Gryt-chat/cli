package config

import "testing"

func TestChannelDefaultsToStable(t *testing.T) {
	store := NewStore(t.TempDir())
	if got := store.Preferences().Channel; got != ChannelStable {
		t.Fatalf("channel = %q", got)
	}
	if tag := store.Preferences().ImageTag(); tag != "latest" {
		t.Fatalf("image tag = %q", tag)
	}
}

func TestBetaChannelChangesTheImageTag(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SetChannel(ChannelBeta); err != nil {
		t.Fatal(err)
	}
	prefs := store.Preferences()
	if !prefs.IsBeta() || prefs.ImageTag() != "latest-beta" {
		t.Fatalf("beta did not take: %#v tag=%q", prefs, prefs.ImageTag())
	}
}

// Anything that is not a channel this CLI knows falls back to stable rather
// than being written through, so a typo cannot leave a machine following a
// channel that does not exist.
func TestAnUnknownChannelFallsBackToStable(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SetChannel("weekly"); err != nil {
		t.Fatal(err)
	}
	if store.Preferences().Channel != ChannelStable {
		t.Fatalf("channel = %q", store.Preferences().Channel)
	}
}

func TestTheChannelReachesTheGeneratedCompose(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SetChannel(ChannelBeta); err != nil {
		t.Fatal(err)
	}
	path, err := store.WriteCompose(NewProfile("Tagged"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := readFile(path)
	if !contains(body, "server:latest-beta") {
		t.Fatalf("the compose did not follow the channel:\n%s", body)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
