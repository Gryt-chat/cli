package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Gryt-chat/cli/internal/config"
	gruntime "github.com/Gryt-chat/cli/internal/runtime"
)

func modelWith(t *testing.T, names ...string) (Model, *gruntime.Fake) {
	t.Helper()
	store := config.NewStore(t.TempDir())
	var profiles []config.Profile
	for _, name := range names {
		profile := config.NewProfile(name)
		if err := store.Save(profile); err != nil {
			t.Fatal(err)
		}
		profiles = append(profiles, profile)
	}
	fake := &gruntime.Fake{Containers: map[string]bool{}}
	m := New(store, fake, "v0.4.1")
	m.profiles = profiles
	return m, fake
}

func TestSharedPiecesAppearAfterTheServers(t *testing.T) {
	m, _ := modelWith(t, "Alpha", "Beta")
	list := m.entries()

	if len(list) != 4 {
		t.Fatalf("expected two servers and two shared pieces, got %d", len(list))
	}
	if list[0].kind != entryServer || list[1].kind != entryServer {
		t.Fatal("servers should come first")
	}
	if list[2].container != config.SFUContainer || list[3].container != config.MinIOContainer {
		t.Fatalf("shared pieces are %q and %q", list[2].container, list[3].container)
	}
}

// Nothing is shared when there is nothing to share it between, and an empty
// dashboard should not offer two rows about infrastructure for no servers.
func TestNoSharedPiecesWithoutServers(t *testing.T) {
	m, _ := modelWith(t)
	if len(m.entries()) != 0 {
		t.Fatalf("an empty dashboard listed %d rows", len(m.entries()))
	}
}

// A shared row has no profile, so every key that acts on a server has to do
// nothing there rather than acting on whichever server happens to be last.
func TestServerActionsDoNothingOnASharedRow(t *testing.T) {
	m, fake := modelWith(t, "Alpha")
	m.selected = 1 // the SFU

	if _, ok := m.selectedProfile(); ok {
		t.Fatal("a shared row answered with a server profile")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'r'})
	if next := updated.(Model); next.busy {
		t.Fatal("restart acted on something from a shared row")
	}
	if fake.States["alpha"] == gruntime.StateRunning {
		t.Fatal("a shared row restarted a server")
	}
}

func TestLogsOnASharedRowReadTheContainer(t *testing.T) {
	m, fake := modelWith(t, "Alpha")
	fake.Log = "sfu is talking"
	m.selected = 1

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'l'})
	if cmd == nil {
		t.Fatal("l did nothing on a shared row")
	}
	if next := updated.(Model); next.mode != modeLogs {
		t.Fatal("l on a shared row did not open the log view")
	}
	msg, ok := cmd().(logsLoaded)
	if !ok || msg.content != "sfu is talking" {
		t.Fatalf("unexpected log message: %#v", msg)
	}
}

// Stopping one of these is not a per-server act: it takes voice and uploads
// away from every server at once, so the message has to say so.
func TestStoppingASharedPieceSaysWhatItCosts(t *testing.T) {
	m, _ := modelWith(t, "Alpha")
	m.selected = 1

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x'})
	if cmd == nil {
		t.Fatal("x did nothing on a shared row")
	}
	done, ok := cmd().(operationDone)
	if !ok {
		t.Fatalf("unexpected message: %#v", cmd())
	}
	if !strings.Contains(done.message, "every server") {
		t.Fatalf("the message does not say what it costs: %q", done.message)
	}
}

func TestSharedRowsReportTheirOwnState(t *testing.T) {
	m, _ := modelWith(t, "Alpha")
	m.containers = map[string]bool{config.SFUContainer: true}

	_, word, _ := m.entryState(m.entries()[1])
	if word != "running" {
		t.Fatalf("a running SFU reads as %q", word)
	}
	if _, word, _ := m.entryState(m.entries()[2]); word != "stopped" {
		t.Fatalf("a stopped object store reads as %q", word)
	}
}
