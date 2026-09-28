package start

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Gryt-chat/cli/internal/config"
	"github.com/Gryt-chat/cli/internal/runtime"
)

func setup(t *testing.T, name string) (*config.Store, config.Profile) {
	t.Helper()
	store := config.NewStore(t.TempDir())
	profile := config.NewProfile(name)
	if err := store.Save(profile); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteCompose(profile); err != nil {
		t.Fatal(err)
	}
	return store, profile
}

func run(t *testing.T, store *config.Store, fake *runtime.Fake, opts Options) (config.Profile, string, error) {
	t.Helper()
	var out strings.Builder
	profile, err := Run(context.Background(), &out, store, fake, opts)
	return profile, out.String(), err
}

func TestStartsTheSharedProjectFirst(t *testing.T) {
	store, profile := setup(t, "Alpha")
	fake := &runtime.Fake{}

	_, out, err := run(t, store, fake, Options{ServerID: profile.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !fake.SharedStarted {
		t.Fatal("the shared voice server was never started")
	}
	if len(fake.Starts) != 1 || fake.Starts[0] != profile.ID {
		t.Fatalf("expected %s started, got %v", profile.ID, fake.Starts)
	}
	if !strings.Contains(out, profile.Name) || !strings.Contains(out, "is up on") {
		t.Fatalf("expected an up notice, got:\n%s", out)
	}
}

func TestRefusesAnUnknownServer(t *testing.T) {
	store, _ := setup(t, "Alpha")
	if _, _, err := run(t, store, &runtime.Fake{}, Options{ServerID: "nope"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a not-found refusal, got %v", err)
	}
}

func TestDockerUnavailableStopsBeforeTouchingTheSharedProject(t *testing.T) {
	store, profile := setup(t, "Alpha")
	fake := &runtime.Fake{Err: errors.New("docker: not running")}

	if _, _, err := run(t, store, fake, Options{ServerID: profile.ID}); err == nil {
		t.Fatal("expected an error")
	}
	if fake.SharedStarted || len(fake.Starts) != 0 {
		t.Fatal("started something although docker was unavailable")
	}
}

func TestAManagerErrorIsSurfaced(t *testing.T) {
	store, profile := setup(t, "Alpha")
	fake := &runtime.Fake{Err: errors.New("boom")}

	if _, _, err := run(t, store, fake, Options{ServerID: profile.ID}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected the manager's error surfaced, got %v", err)
	}
}
