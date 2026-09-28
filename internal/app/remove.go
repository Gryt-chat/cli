package app

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Gryt-chat/cli/internal/config"
	"github.com/Gryt-chat/cli/internal/remove"
)

// removeKey handles the remove screen, where the server's id has to be typed before
// enter does anything. The same question gryt remove asks on the command line.
func (m Model) removeKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	profile, ok := m.selectedProfile()
	if !ok {
		m.mode = modeDashboard
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode, m.removeInput = modeDashboard, ""
		return m, nil
	case "backspace":
		if m.removeInput != "" {
			m.removeInput = m.removeInput[:len(m.removeInput)-1]
		}
		return m, nil
	case "enter":
		if m.removeInput != profile.ID {
			return m, nil
		}
		m.mode, m.removeInput = modeDashboard, ""
		return m.startWork(profile.ID), m.removeServer(profile)
	}
	if text := key.Text; len(text) == 1 && len(m.removeInput) < 64 {
		m.removeInput += text
	}
	return m, nil
}

func (m Model) removeServer(profile config.Profile) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		result, err := remove.Run(ctx, io.Discard, m.store, m.runtime, nil, remove.Options{ServerID: profile.ID, Yes: true})
		if err != nil {
			return operationDone{err: err, key: profile.ID}
		}
		message := "Removed " + profile.Name
		if result.Last {
			message += ", and the shared voice server with it"
		}
		return operationDone{message: message, key: profile.ID}
	}
}

func (m Model) viewRemove() string {
	profile, ok := m.selectedProfile()
	if !ok {
		return m.viewDashboard()
	}
	head := m.header(m.styles.danger.Render("remove " + profile.Name))
	lines := []string{
		m.styles.title.Render("Delete " + profile.Name + " for good?"),
		"",
		fmt.Sprintf("  Its containers, gryt-%s and gryt-%s-image-worker", profile.ID, profile.ID),
		"  " + m.store.ServerDir(profile.ID) + ", with its database, uploads and settings",
	}
	if len(m.profiles) == 1 {
		lines = append(lines, "  The shared voice server too, since no other server here uses it")
	}
	lines = append(lines, "",
		m.styles.muted.Render("There's no undo. Type "+profile.ID+" and press enter to delete it."),
		"",
		"  › "+m.removeInput+m.styles.accent.Render("▏"),
	)
	if m.removeInput != "" && !strings.HasPrefix(profile.ID, m.removeInput) {
		lines = append(lines, "", m.styles.danger.Render("  That isn't "+profile.ID+"."))
	}
	footer := m.styles.footer.Width(m.width).Render(" enter delete   esc cancel")
	body := strings.Join(lines, "\n")
	return head + "\n" + body + "\n" + footer
}
