package remove

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gryt-chat/cli/internal/config"
	"github.com/Gryt-chat/cli/internal/runtime"
)

// setup saves each named server with its generated files, and the shared project beside them.
func setup(t *testing.T, names ...string) *config.Store {
	t.Helper()
	store := config.NewStore(t.TempDir())
	for _, name := range names {
		profile := config.NewProfile(name)
		if err := store.Save(profile); err != nil {
			t.Fatal(err)
		}
		if _, err := store.WriteCompose(profile); err != nil {
			t.Fatal(err)
		}
		data := filepath.Join(store.ServerDir(profile.ID), "data")
		if err := os.MkdirAll(filepath.Join(data, "gryt"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(data, "gryt.db"), []byte("db"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.WriteSharedCompose(); err != nil {
		t.Fatal(err)
	}
	return store
}

func answer(text string) Ask {
	return func(string) (string, error) { return text, nil }
}

func run(t *testing.T, store *config.Store, fake *runtime.Fake, ask Ask, opts Options) (Result, string, error) {
	t.Helper()
	var out strings.Builder
	result, err := Run(context.Background(), &out, store, fake, ask, opts)
	return result, out.String(), err
}

func TestAWrongAnswerRemovesNothing(t *testing.T) {
	store := setup(t, "Alpha")
	fake := &runtime.Fake{}

	_, _, err := run(t, store, fake, answer("yes\n"), Options{ServerID: "alpha"})
	if !errors.Is(err, ErrDeclined) {
		t.Fatalf("expected ErrDeclined, got %v", err)
	}
	if len(fake.Removed) != 0 || !exists(store.ServerDir("alpha")) {
		t.Fatalf("a declined remove touched something: %v", fake.Removed)
	}
}

func TestAClosedStdinRemovesNothing(t *testing.T) {
	store := setup(t, "Alpha")
	fake := &runtime.Fake{}
	eof := func(string) (string, error) { return "", errors.New("EOF") }

	if _, _, err := run(t, store, fake, eof, Options{ServerID: "alpha"}); !errors.Is(err, ErrDeclined) {
		t.Fatalf("expected ErrDeclined, got %v", err)
	}
	if !exists(store.ServerDir("alpha")) {
		t.Fatal("the server folder went without an answer")
	}
}

func TestRefusesAnUnknownServer(t *testing.T) {
	store := setup(t, "Alpha")
	if _, _, err := run(t, store, &runtime.Fake{}, answer("nope\n"), Options{ServerID: "nope"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a not-found refusal, got %v", err)
	}
}

func TestRemovingOneOfTwoLeavesTheSharedServices(t *testing.T) {
	store := setup(t, "Alpha", "Beta")
	fake := &runtime.Fake{}

	result, out, err := run(t, store, fake, answer("beta\n"), Options{ServerID: "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Last {
		t.Fatal("reported beta as the last server")
	}
	if exists(store.ServerDir("beta")) {
		t.Fatal("servers/beta is still there")
	}
	if !exists(store.ServerDir("alpha")) || !exists(store.SharedDir()) {
		t.Fatal("removing beta took alpha or the shared project with it")
	}
	if fake.SharedGone || len(fake.Removed) != 1 || fake.Removed[0] != store.ServerDir("beta") {
		t.Fatalf("expected only beta's project down, got %v shared=%v", fake.Removed, fake.SharedGone)
	}
	if strings.Contains(out, "shared services go too") {
		t.Fatalf("warned about the shared services when they stay:\n%s", out)
	}
}

func TestTheLastServerTakesTheSharedServicesWithIt(t *testing.T) {
	store := setup(t, "Alpha")
	fake := &runtime.Fake{}

	result, out, err := run(t, store, fake, nil, Options{ServerID: "alpha", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Last || !fake.SharedGone {
		t.Fatalf("the shared project stayed: last=%v gone=%v", result.Last, fake.SharedGone)
	}
	if exists(store.ServerDir("alpha")) || exists(store.SharedDir()) || exists(store.Root()) {
		t.Fatal("a folder was left behind")
	}
	for _, want := range []string{config.SFUContainer, "shared services go too", "Removed Alpha"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output never mentioned %q:\n%s", want, out)
		}
	}
}

// On Linux the data folder belongs to uid 1001, and the user running gryt cannot delete it.
func TestAFolderTheUserCannotDeleteGoesThroughDocker(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can delete anything, so there is nothing to fall back from")
	}
	store := setup(t, "Alpha", "Beta")
	data := filepath.Join(store.ServerDir("alpha"), "data")
	if err := os.Chmod(filepath.Join(data, "gryt"), 0o500); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "gryt", "locked"), nil, 0o644); err == nil {
		t.Skip("this filesystem ignores the mode, so the folder is not locked")
	}
	if err := os.Chmod(data, 0o500); err != nil {
		t.Fatal(err)
	}
	fake := &runtime.Fake{}

	if _, _, err := run(t, store, fake, answer("alpha\n"), Options{ServerID: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if len(fake.DeletedAsRoot) != 1 || fake.DeletedAsRoot[0] != data {
		t.Fatalf("expected %s deleted through docker, got %v", data, fake.DeletedAsRoot)
	}
	if exists(store.ServerDir("alpha")) {
		t.Fatal("servers/alpha is still there")
	}
}

func TestAFailedComposeDownLeavesTheFolder(t *testing.T) {
	store := setup(t, "Alpha")
	fake := &runtime.Fake{Err: errors.New("docker compose: boom")}

	// Available fails first with the same error, which is also a refusal to touch disk.
	if _, _, err := run(t, store, fake, answer("alpha\n"), Options{ServerID: "alpha"}); err == nil {
		t.Fatal("expected an error")
	}
	if !exists(filepath.Join(store.ServerDir("alpha"), "profile.json")) {
		t.Fatal("the folder went although docker failed")
	}
}
