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
	// Stop is only offered on something that is running.
	m.containers = map[string]bool{config.SFUContainer: true}

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

// Start used to be accepted on a running server: it ran compose up again and
// reported "Start X" as though something had happened.
func TestStartIsNotOfferedOnSomethingAlreadyRunning(t *testing.T) {
	m, fake := modelWith(t, "Alpha")
	m.states = map[string]gruntime.State{"alpha": gruntime.StateRunning}

	if _, cmd := m.Update(tea.KeyPressMsg{Code: 's'}); cmd != nil {
		t.Fatal("start was accepted on a running server")
	}
	if fake.SharedStarted {
		t.Fatal("start did work on a running server")
	}
	if strings.Contains(m.dashboardKeys(), "s start") {
		t.Fatalf("start is still offered: %q", m.dashboardKeys())
	}
	if !strings.Contains(m.dashboardKeys(), "x stop") {
		t.Fatalf("stop should be offered on a running server: %q", m.dashboardKeys())
	}
}

func TestStopIsNotOfferedOnSomethingStopped(t *testing.T) {
	m, _ := modelWith(t, "Alpha")

	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'x'}); cmd != nil {
		t.Fatal("stop was accepted on a stopped server")
	}
	if strings.Contains(m.dashboardKeys(), "x stop") {
		t.Fatalf("stop is still offered: %q", m.dashboardKeys())
	}
}

// An unreadable state refuses nothing: the operator may need to start or stop
// it precisely to find out what is going on.
func TestAnUnknownStateOffersEverything(t *testing.T) {
	m, _ := modelWith(t, "Alpha")
	m.states = map[string]gruntime.State{"alpha": gruntime.StateUnknown}

	keys := m.dashboardKeys()
	for _, want := range []string{"s start", "x stop", "r restart"} {
		if !strings.Contains(keys, want) {
			t.Fatalf("%q missing from %q", want, keys)
		}
	}
}

// The freeze: one global flag dropped every keypress while any operation ran,
// for the whole length of a docker command.
func TestNavigationKeepsWorkingWhileARowIsBusy(t *testing.T) {
	m, _ := modelWith(t, "Alpha", "Beta")
	m = m.startWork("alpha")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if next := updated.(Model); next.selected != 1 {
		t.Fatal("the dashboard ignored an arrow key while a row was working")
	}

	// And a second row can be acted on while the first is still going.
	next := updated.(Model)
	if _, cmd := next.Update(tea.KeyPressMsg{Code: 's'}); cmd == nil {
		t.Fatal("a second server could not be started while the first was working")
	}
}

func TestASecondActionOnTheSameRowIsRefused(t *testing.T) {
	m, _ := modelWith(t, "Alpha")
	m = m.startWork("alpha")

	if _, cmd := m.Update(tea.KeyPressMsg{Code: 's'}); cmd != nil {
		t.Fatal("a row already working accepted another action")
	}
	if !strings.Contains(m.dashboardKeys(), "working") {
		t.Fatalf("the footer does not say the row is working: %q", m.dashboardKeys())
	}
}

func TestFinishingOneRowLeavesOthersWorking(t *testing.T) {
	m, _ := modelWith(t, "Alpha", "Beta")
	m = m.startWork("alpha")
	m = m.startWork("beta")

	updated, _ := m.Update(operationDone{message: "Start Alpha", key: "alpha"})
	next := updated.(Model)
	if next.working["alpha"] {
		t.Fatal("alpha should have stopped working")
	}
	if !next.working["beta"] {
		t.Fatal("beta stopped working because alpha finished")
	}
}

// The working state belongs on the row, not in one banner, so several servers
// starting at once are each visible doing it.
func TestTheRowItselfShowsThatItIsWorking(t *testing.T) {
	m, _ := modelWith(t, "Alpha", "Beta")
	m.width, m.height = 120, 24
	m = m.startWork("alpha")

	view := m.viewDashboard()
	if !strings.Contains(view, "working") {
		t.Fatalf("no working state on the table:\n%s", view)
	}

	// And only on the row doing the work.
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Beta") && strings.Contains(line, "working") {
			t.Fatalf("a row that is not working says it is: %q", line)
		}
	}
}

// Editing a running server rewrote .env and compose.yaml, reported "Saved",
// and left the container running the old values.
func TestSavingAChangeToARunningServerAppliesIt(t *testing.T) {
	m, fake := modelWith(t, "Alpha")
	profile := m.profiles[0]
	m.states = map[string]gruntime.State{profile.ID: gruntime.StateRunning}

	// Write the files once so there is a "before" to differ from.
	if _, err := m.store.WriteEnv(profile); err != nil {
		t.Fatal(err)
	}
	if _, err := m.store.WriteCompose(profile); err != nil {
		t.Fatal(err)
	}

	profile.VoiceMaxUsers = 12
	msg, ok := m.saveProfile(profile)().(operationDone)
	if !ok {
		t.Fatalf("unexpected message type")
	}
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if !strings.Contains(msg.message, "restarted it to apply") {
		t.Fatalf("the save did not say it applied the change: %q", msg.message)
	}
	if fake.States[profile.ID] != gruntime.StateRunning {
		t.Fatal("the server was not brought back up")
	}
}

// A save that changes nothing must not bounce a running server.
func TestSavingWithNoChangeLeavesARunningServerAlone(t *testing.T) {
	m, fake := modelWith(t, "Alpha")
	profile := m.profiles[0]
	m.states = map[string]gruntime.State{profile.ID: gruntime.StateRunning}
	if _, err := m.store.WriteEnv(profile); err != nil {
		t.Fatal(err)
	}
	if _, err := m.store.WriteCompose(profile); err != nil {
		t.Fatal(err)
	}

	fake.States = map[string]gruntime.State{}
	msg := m.saveProfile(profile)().(operationDone)
	if strings.Contains(msg.message, "restarted") {
		t.Fatalf("an unchanged save restarted the server: %q", msg.message)
	}
	if _, touched := fake.States[profile.ID]; touched {
		t.Fatal("an unchanged save touched the container")
	}
}

// A stopped server is only written, never started, by a save.
func TestSavingAStoppedServerDoesNotStartIt(t *testing.T) {
	m, fake := modelWith(t, "Alpha")
	profile := m.profiles[0]
	profile.VoiceMaxUsers = 9

	msg := m.saveProfile(profile)().(operationDone)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if _, started := fake.States[profile.ID]; started {
		t.Fatal("saving a stopped server started it")
	}
}
