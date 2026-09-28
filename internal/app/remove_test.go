package app

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Gryt-chat/cli/internal/config"
	gruntime "github.com/Gryt-chat/cli/internal/runtime"
)

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		updated, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(Model)
	}
	return m
}

func TestRemoveNeedsTheServerIDTyped(t *testing.T) {
	store := config.NewStore(t.TempDir())
	profile := config.NewProfile("Doomed")
	if err := store.Save(profile); err != nil {
		t.Fatal(err)
	}
	fake := &gruntime.Fake{}
	m := New(store, fake, "v0.1.0")
	m.profiles = []config.Profile{profile}

	m = typeText(t, m, "D")
	if m.mode != modeRemove {
		t.Fatalf("D did not open the remove screen, mode %v", m.mode)
	}
	if view := m.viewRemove(); !containsAll(view, "Doomed", "doomed", store.ServerDir("doomed")) {
		t.Fatalf("the remove screen does not say what goes:\n%s", view)
	}

	m = typeText(t, m, "doom")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil || m.mode != modeRemove {
		t.Fatal("enter on a partial id removed something")
	}

	m = typeText(t, m, "ed")
	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil || m.mode != modeDashboard {
		t.Fatal("enter on the full id did nothing")
	}
	done, ok := cmd().(operationDone)
	if !ok || done.err != nil {
		t.Fatalf("remove failed: %#v", done)
	}
	if _, err := os.Stat(store.ServerDir("doomed")); !os.IsNotExist(err) {
		t.Fatalf("the server folder is still there: %v", err)
	}
}

func TestEscLeavesTheRemoveScreenWithoutRemoving(t *testing.T) {
	store := config.NewStore(t.TempDir())
	profile := config.NewProfile("Kept")
	if err := store.Save(profile); err != nil {
		t.Fatal(err)
	}
	m := New(store, &gruntime.Fake{}, "v0.1.0")
	m.profiles = []config.Profile{profile}

	m = typeText(t, m, "Dkept")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if cmd != nil || m.mode != modeDashboard {
		t.Fatal("esc did not cancel")
	}
	if _, err := os.Stat(store.ServerDir("kept")); err != nil {
		t.Fatal(err)
	}
}
