package pull

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Gryt-chat/cli/internal/config"
	"github.com/Gryt-chat/cli/internal/runtime"
)

func newest(tag string) Latest {
	return func(context.Context) (string, error) { return tag, nil }
}

// setup writes one saved server into a temporary root and returns the store beside it.
func setup(t *testing.T) (*config.Store, config.Profile) {
	t.Helper()
	store := config.NewStore(t.TempDir())
	profile := config.NewProfile("Pull Me")
	profile.Port = 5099
	if err := store.Save(profile); err != nil {
		t.Fatal(err)
	}
	return store, profile
}

// running is a fake with the server and the shared SFU up, reporting one version.
func running(version string) *runtime.Fake {
	return &runtime.Fake{
		Containers: map[string]bool{"gryt-pull-me": true, config.SFUContainer: true},
		Env:        map[string]string{"SERVER_VERSION": version},
	}
}

func run(t *testing.T, store *config.Store, fake *runtime.Fake, latest Latest, opts Options) (string, error) {
	t.Helper()
	var out strings.Builder
	err := Run(context.Background(), &out, store, fake, latest, opts)
	return out.String(), err
}

func TestRefusesAServerThatIsNotRunning(t *testing.T) {
	store, _ := setup(t)
	fake := &runtime.Fake{Containers: map[string]bool{}}

	_, err := run(t, store, fake, newest("v1.9.0"), Options{ServerID: "pull-me"})
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected a refusal naming the state, got %v", err)
	}
	if len(fake.Pulls) != 0 {
		t.Fatalf("a stopped server still pulled: %v", fake.Pulls)
	}
}

func TestRefusesAnUnknownServer(t *testing.T) {
	store, _ := setup(t)

	_, err := run(t, store, running("1.8.0"), newest("v1.9.0"), Options{ServerID: "nope"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a not-found refusal, got %v", err)
	}
}

func TestStopsWhenNothingIsNewer(t *testing.T) {
	store, _ := setup(t)
	fake := running("1.9.0")
	fake.ImageRefs = map[string]string{"gryt-pull-me": "ghcr.io/gryt-chat/server:latest"}

	out, err := run(t, store, fake, newest("v1.9.0"), Options{ServerID: "pull-me"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Pulls) != 0 || len(fake.Starts) != 0 {
		t.Fatalf("an up-to-date server was pulled or recreated: %v %v", fake.Pulls, fake.Starts)
	}
	for _, want := range []string{"newest server release", "--force"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output never mentioned %q:\n%s", want, out)
		}
	}
}

func TestForcePullsWithoutAskingGitHub(t *testing.T) {
	store, _ := setup(t)
	fake := running("1.9.0")
	asked := false
	latest := func(context.Context) (string, error) {
		asked = true
		return "v1.9.0", nil
	}

	if _, err := run(t, store, fake, latest, Options{ServerID: "pull-me", Force: true}); err != nil {
		t.Fatal(err)
	}
	if asked {
		t.Fatal("--force still asked GitHub which release is newest")
	}
	if len(fake.Pulls) != 1 {
		t.Fatalf("expected the server's own project pulled, got %v", fake.Pulls)
	}
}

func TestReportsTheVersionMoving(t *testing.T) {
	store, _ := setup(t)
	fake := running("1.8.0")
	fake.AfterStart = map[string]string{"SERVER_VERSION": "1.9.0"}

	out, err := run(t, store, fake, newest("v1.9.0"), Options{ServerID: "pull-me"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1.8.0 → 1.9.0") {
		t.Fatalf("the before and after version never appeared:\n%s", out)
	}
	if len(fake.Starts) != 1 {
		t.Fatalf("expected one recreate, got %v", fake.Starts)
	}
}

func TestPullsEverythingBeforeRecreatingAnything(t *testing.T) {
	store, _ := setup(t)
	fake := running("1.8.0")
	fake.PullErr = errors.New("registry said no")

	out, err := run(t, store, fake, newest("v1.9.0"), Options{ServerID: "pull-me"})
	if err == nil {
		t.Fatal("a failed pull reported success")
	}
	if len(fake.Starts) != 0 {
		t.Fatalf("a failed pull still recreated the server: %v", fake.Starts)
	}
	if !strings.Contains(out, "Nothing was recreated") || !strings.Contains(out, "still running 1.8.0") {
		t.Fatalf("the failure never said what the server is left on:\n%s", out)
	}
}

func TestLeavesTheSharedVoiceServerAlone(t *testing.T) {
	store, _ := setup(t)
	fake := running("1.8.0")

	out, err := run(t, store, fake, newest("v1.9.0"), Options{ServerID: "pull-me"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Pulls) != 1 {
		t.Fatalf("one server's pull touched the shared project: %v", fake.Pulls)
	}
	if fake.SharedStarted {
		t.Fatal("one server's pull recreated the shared project")
	}
	if !strings.Contains(out, "gryt pull --shared") {
		t.Fatalf("the output never said how to move the voice server:\n%s", out)
	}
}

func TestSharedRefusesWhenTheVoiceServerIsDown(t *testing.T) {
	store, _ := setup(t)
	fake := running("1.8.0")
	fake.Containers[config.SFUContainer] = false

	_, err := run(t, store, fake, newest("v1.9.0"), Options{Shared: true})
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected a refusal naming the state, got %v", err)
	}
	if len(fake.Pulls) != 0 {
		t.Fatalf("a stopped shared project still pulled: %v", fake.Pulls)
	}
}

func TestSharedReportsTheImageMoving(t *testing.T) {
	store, _ := setup(t)
	fake := running("1.8.0")
	fake.Images = map[string]string{config.SFUContainer: "sha256:1111111111119999"}
	fake.AfterShared = map[string]string{config.SFUContainer: "sha256:2222222222228888"}

	out, err := run(t, store, fake, newest("v1.9.0"), Options{Shared: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "111111111111 → 222222222222") {
		t.Fatalf("the before and after image never appeared:\n%s", out)
	}
	if !fake.SharedStarted {
		t.Fatal("the shared project was pulled but never recreated")
	}
}

func TestSharedSaysWhenNothingMoved(t *testing.T) {
	store, _ := setup(t)
	fake := running("1.8.0")
	fake.Images = map[string]string{config.SFUContainer: "sha256:1111111111119999"}

	out, err := run(t, store, fake, newest("v1.9.0"), Options{Shared: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "nothing was replaced") {
		t.Fatalf("an unchanged shared project did not say so:\n%s", out)
	}
}

func TestPullsAnImageTooOldToReportAVersion(t *testing.T) {
	store, _ := setup(t)
	fake := running("")
	asked := false
	latest := func(context.Context) (string, error) {
		asked = true
		return "v1.9.0", nil
	}

	out, err := run(t, store, fake, latest, Options{ServerID: "pull-me"})
	if err != nil {
		t.Fatal(err)
	}
	if asked {
		t.Fatal("a server with no version to compare still asked GitHub")
	}
	if len(fake.Pulls) == 0 || !strings.Contains(out, "reports no version") {
		t.Fatalf("an unversioned image was not pulled:\n%s", out)
	}
}

func TestPullsAServerLeftOnTheOtherChannelsTag(t *testing.T) {
	store, _ := setup(t)
	if err := store.SetChannel(config.ChannelBeta); err != nil {
		t.Fatal(err)
	}
	fake := running("1.9.0")
	fake.ImageRefs = map[string]string{"gryt-pull-me": "ghcr.io/gryt-chat/server:latest"}

	out, err := run(t, store, fake, newest("v1.9.0"), Options{ServerID: "pull-me"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Pulls) != 1 {
		t.Fatalf("a server on the wrong channel's tag was told there was nothing to pull:\n%s", out)
	}
	if !strings.Contains(out, "points at latest-beta") {
		t.Fatalf("the output never said why it pulled:\n%s", out)
	}
}

func TestCarriesAFailedReleaseCheck(t *testing.T) {
	store, _ := setup(t)
	fake := running("1.8.0")
	latest := func(context.Context) (string, error) { return "", errors.New("no route to host") }

	_, err := run(t, store, fake, latest, Options{ServerID: "pull-me"})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("an unreachable releases API did not point at --force: %v", err)
	}
	if len(fake.Pulls) != 0 {
		t.Fatalf("a failed check still pulled: %v", fake.Pulls)
	}
}
