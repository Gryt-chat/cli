package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Gryt-chat/cli/internal/config"
	"github.com/Gryt-chat/cli/internal/management"
)

// The settings a server keeps in its own database, rather than in the
// environment its container was started with.
//
// These could not be changed from here at all: they are authorised by
// ownership, so managing a server meant being its owner in a client, even on a
// machine you administer. They are reached through the server's management API
// now, which is also what makes a change take effect — turning discovery off
// has to withdraw the mDNS advertisement, and only the server can do that.
type settingsLoaded struct {
	settings *management.Settings
	err      error
}

type settingRow struct {
	key   string
	label string
	// help explains what the setting does to somebody who has not read the
	// docs, which is everybody the first time.
	help string
	// options in the order they cycle. A toggle is two options.
	options []string
	value   string
}

func (m Model) settingRows() []settingRow {
	s := m.settings
	if s == nil {
		return nil
	}
	yesNo := []string{"on", "off"}
	onOff := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	return []settingRow{
		{
			key: "joinPolicy", label: "Who can join",
			help:    "Invite means somebody needs a link. Open means anybody who can reach the address.",
			options: []string{"invite", "open"}, value: s.JoinPolicy,
		},
		{
			key: "discoverable", label: "LAN discovery",
			help:    "Advertises the server over mDNS, so clients on this network list it without being given the address.",
			options: yesNo, value: onOff(s.Discoverable),
		},
		{
			key: "lanOpen", label: "Open on the LAN",
			help:    "Lets anybody already on this network join without an invite.",
			options: yesNo, value: onOff(s.LANOpen),
		},
		{
			key: "profanityMode", label: "Profanity filter",
			help:    "off leaves messages alone · flag marks them · censor hides the word · block refuses the message.",
			options: []string{"off", "flag", "censor", "block"}, value: s.ProfanityMode,
		},
	}
}

// loadSettings asks the server what its settings are.
func (m Model) loadSettings(profile config.Profile) tea.Cmd {
	client := management.Client{Port: profile.AdminPort, Token: profile.AdminToken}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		settings, err := client.Get(ctx)
		return settingsLoaded{settings: settings, err: err}
	}
}

// applySetting sends one key. Only that key: a patch carrying everything this
// side is holding would push a stale value back over anything changed
// elsewhere since it was read.
func (m Model) applySetting(profile config.Profile, key string, value any) tea.Cmd {
	client := management.Client{Port: profile.AdminPort, Token: profile.AdminToken}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		settings, err := client.Patch(ctx, map[string]any{key: value})
		return settingsLoaded{settings: settings, err: err}
	}
}

// cycle moves a setting to its next value and returns what to send.
func cycle(row settingRow) (display string, send any) {
	next := row.options[0]
	for i, option := range row.options {
		if option == row.value {
			next = row.options[(i+1)%len(row.options)]
			break
		}
	}
	if len(row.options) == 2 && row.options[0] == "on" {
		return next, next == "on"
	}
	return next, next
}

func (m Model) viewSettings() string {
	profile, ok := m.selectedProfile()
	if !ok {
		return m.viewDashboard()
	}
	head := m.header(m.styles.muted.Render("SETTINGS"))
	footer := m.styles.footer.Width(m.width).Render(" ↑/↓ select   space change   esc back")

	lines := []string{"", "  " + m.styles.title.Render(profile.Name)}

	switch {
	case m.settingsErr != "":
		lines = append(lines, "", "  "+m.styles.warning.Render(m.settingsErr))
		// These live in the running server, so there is nothing to show and
		// nothing to change while it is stopped. Say which it is.
		if errors.Is(m.settingsErrKind, management.ErrUnreachable) {
			lines = append(lines, "", "  "+m.styles.muted.Render("Start it with s, then come back."))
		}
	case m.settings == nil:
		lines = append(lines, "", "  "+m.styles.muted.Render("Reading settings…"))
	default:
		rows := m.settingRows()
		for i, row := range rows {
			marker := "  "
			label := m.styles.muted.Render(pad(row.label, 20))
			value := m.styles.muted.Render(row.value)
			if i == m.settingsCursor {
				marker = "› "
				label = m.styles.strong.Render(pad(row.label, 20))
				value = m.styles.accent.Render(row.value)
			}
			lines = append(lines, marker+label+value)
			if i == m.settingsCursor {
				lines = append(lines, "  "+m.styles.muted.Render(wrapText(row.help, max(30, min(72, m.width-4)))))
			}
		}
		if m.settingsBusy {
			lines = append(lines, "", "  "+m.styles.accent.Render("Applying…"))
		}
	}

	body := lipgloss.NewStyle().Height(max(4, m.height-2)).Render(strings.Join(lines, "\n"))
	return head + "\n" + body + "\n" + footer
}

// settingsKey handles the settings screen's own keys.
func (m Model) settingsKey(key string, profile config.Profile) (Model, tea.Cmd) {
	rows := m.settingRows()
	switch key {
	case "esc", "q":
		m.mode = modeDashboard
		return m, nil
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case "down", "j":
		if m.settingsCursor < len(rows)-1 {
			m.settingsCursor++
		}
	case " ", "space", "left", "right", "h", "l", "enter":
		if len(rows) == 0 || m.settingsBusy {
			return m, nil
		}
		row := rows[m.settingsCursor]
		display, send := cycle(row)
		// Shown immediately, then confirmed or corrected by what the server
		// returns, so the screen never sits still while a change is in flight.
		m.settings = applyLocally(m.settings, row.key, display)
		m.settingsBusy = true
		m.settingsErr = ""
		return m, m.applySetting(profile, row.key, send)
	}
	return m, nil
}

// applyLocally mirrors a change into the copy on screen.
func applyLocally(s *management.Settings, key, display string) *management.Settings {
	if s == nil {
		return nil
	}
	next := *s
	switch key {
	case "joinPolicy":
		next.JoinPolicy = display
	case "discoverable":
		next.Discoverable = display == "on"
	case "lanOpen":
		next.LANOpen = display == "on"
	case "profanityMode":
		next.ProfanityMode = display
	}
	return &next
}

func settingsErrorText(err error) string {
	switch {
	case errors.Is(err, management.ErrUnsupported):
		return "This server has no management API. Update its image and restart it."
	case errors.Is(err, management.ErrUnreachable):
		return "The server is not answering."
	default:
		return fmt.Sprintf("%v", err)
	}
}
