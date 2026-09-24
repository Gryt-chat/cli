package autoupdate

import (
	"testing"

	"github.com/Gryt-chat/cli/internal/config"
)

func TestContainersSharedSFUPlusEachProfile(t *testing.T) {
	profiles := []config.Profile{{ID: "alpha"}, {ID: "beta"}}
	got := Containers(profiles)
	want := []string{"gryt-sfu", "gryt-alpha", "gryt-alpha-image-worker", "gryt-beta", "gryt-beta-image-worker"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestContainersNoProfilesIsJustSFU(t *testing.T) {
	got := Containers(nil)
	if len(got) != 1 || got[0] != "gryt-sfu" {
		t.Fatalf("got %v, want [gryt-sfu]", got)
	}
}

func TestEnvFileContentsHasNoStacks(t *testing.T) {
	got := EnvFileContents([]config.Profile{{ID: "alpha"}})
	want := "GRYT_STACKS=\nGRYT_CONTAINERS=gryt-sfu gryt-alpha gryt-alpha-image-worker\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
