package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Gryt-chat/cli/internal/config"
	gruntime "github.com/Gryt-chat/cli/internal/runtime"
)

type mode int

const (
	modeDashboard mode = iota
	modeWizard
	modeLogs
)

type profilesLoaded struct {
	profiles []config.Profile
	err      error
}
type statusesLoaded struct{ states map[string]gruntime.State }
type operationDone struct {
	message string
	err     error
}
type logsLoaded struct {
	profile string
	content string
	err     error
}

type Model struct {
	store    *config.Store
	runtime  gruntime.Manager
	styles   styles
	profiles []config.Profile
	states   map[string]gruntime.State
	selected int
	width    int
	height   int
	mode     mode
	wizard   wizard
	logs     string
	busy     bool
	notice   string
	err      string
}

func New(store *config.Store, manager gruntime.Manager) Model {
	return Model{store: store, runtime: manager, styles: newStyles(), states: map[string]gruntime.State{}, width: 100, height: 30}
}

func (m Model) Init() tea.Cmd { return m.loadProfiles() }

func (m Model) loadProfiles() tea.Cmd {
	return func() tea.Msg {
		profiles, err := m.store.List()
		return profilesLoaded{profiles: profiles, err: err}
	}
}

func (m Model) loadStatuses() tea.Cmd {
	profiles := append([]config.Profile(nil), m.profiles...)
	return func() tea.Msg {
		states := make(map[string]gruntime.State, len(profiles))
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		for _, profile := range profiles {
			states[profile.ID] = m.runtime.Status(ctx, profile)
		}
		return statusesLoaded{states: states}
	}
}

func (m Model) saveProfile(profile config.Profile) tea.Cmd {
	return func() tea.Msg {
		if err := m.store.Save(profile); err != nil {
			return operationDone{err: err}
		}
		if _, err := m.store.WriteEnv(profile); err != nil {
			return operationDone{err: err}
		}
		if _, err := m.store.WriteCompose(profile); err != nil {
			return operationDone{err: err}
		}
		return operationDone{message: fmt.Sprintf("Saved %s", profile.Name)}
	}
}

func (m Model) runOperation(action string, profile config.Profile) tea.Cmd {
	dir := m.store.ServerDir(profile.ID)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		var err error
		switch action {
		case "start":
			err = m.runtime.Start(ctx, profile, dir)
		case "stop":
			err = m.runtime.Stop(ctx, profile, dir)
		case "restart":
			err = m.runtime.Restart(ctx, profile, dir)
		}
		if err != nil {
			return operationDone{err: err}
		}
		return operationDone{message: fmt.Sprintf("%s %s", strings.Title(action), profile.Name)}
	}
}

func (m Model) loadLogs(profile config.Profile) tea.Cmd {
	dir := m.store.ServerDir(profile.ID)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		logs, err := m.runtime.Logs(ctx, profile, dir, 120)
		return logsLoaded{profile: profile.Name, content: logs, err: err}
	}
}

func (m Model) selectedProfile() (config.Profile, bool) {
	if len(m.profiles) == 0 || m.selected < 0 || m.selected >= len(m.profiles) {
		return config.Profile{}, false
	}
	return m.profiles[m.selected], true
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case profilesLoaded:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.profiles = msg.profiles
		if m.selected >= len(m.profiles) {
			m.selected = max(0, len(m.profiles)-1)
		}
		return m, m.loadStatuses()
	case statusesLoaded:
		m.states = msg.states
	case operationDone:
		m.busy = false
		if msg.err != nil {
			m.err, m.notice = msg.err.Error(), ""
		} else {
			m.notice, m.err = msg.message, ""
		}
		return m, tea.Batch(m.loadProfiles(), tea.Tick(800*time.Millisecond, func(time.Time) tea.Msg { return statusesLoaded{states: m.states} }))
	case logsLoaded:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err.Error()
			m.mode = modeDashboard
		} else {
			m.logs = msg.content
			m.notice = "Logs · " + msg.profile
			m.mode = modeLogs
		}
	}

	key, isKey := msg.(tea.KeyPressMsg)
	if m.mode == modeWizard {
		if isKey {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.mode, m.err = modeDashboard, ""
				return m, nil
			case "shift+tab":
				return m, m.wizard.previous()
			case "enter":
				if m.wizard.onLastStep() {
					profile, err := m.wizard.profile()
					if err != nil {
						m.wizard.err = err.Error()
						return m, nil
					}
					m.busy, m.mode = true, modeDashboard
					return m, m.saveProfile(profile)
				}
				return m, m.wizard.next()
			}
		}
		return m, m.wizard.update(msg)
	}
	if m.mode == modeLogs {
		if isKey && (key.String() == "esc" || key.String() == "q") {
			m.mode, m.logs = modeDashboard, ""
		}
		return m, nil
	}
	if !isKey {
		return m, nil
	}
	if m.busy && key.String() != "ctrl+c" {
		return m, nil
	}

	profile, hasProfile := m.selectedProfile()
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.profiles)-1 {
			m.selected++
		}
	case "n":
		m.mode, m.wizard, m.notice, m.err = modeWizard, newWizard(), "", ""
		return m, m.wizard.focus()
	case "e":
		if hasProfile {
			m.mode, m.wizard, m.notice, m.err = modeWizard, wizardFromProfile(profile), "", ""
			return m, m.wizard.focus()
		}
	case "s":
		if hasProfile {
			m.busy = true
			return m, m.runOperation("start", profile)
		}
	case "x":
		if hasProfile {
			m.busy = true
			return m, m.runOperation("stop", profile)
		}
	case "r":
		if hasProfile {
			m.busy = true
			return m, m.runOperation("restart", profile)
		}
	case "l":
		if hasProfile {
			m.busy = true
			return m, m.loadLogs(profile)
		}
	case "g":
		return m, m.loadStatuses()
	}
	return m, nil
}

func (m Model) View() tea.View {
	var content string
	switch m.mode {
	case modeWizard:
		content = m.viewWizard()
	case modeLogs:
		content = m.viewLogs()
	default:
		content = m.viewDashboard()
	}
	view := tea.NewView(m.styles.base.Width(m.width).Height(m.height).Render(content))
	view.AltScreen = true
	view.WindowTitle = "Gryt server manager"
	return view
}

func (m Model) header(section string) string {
	left := m.styles.brand.Render("gryt") + m.styles.muted.Render("  server manager")
	right := m.styles.muted.Render(section)
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	return m.styles.header.Width(m.width).Render(left + strings.Repeat(" ", gap) + right)
}

func (m Model) viewDashboard() string {
	header := m.header("SERVERS")
	footer := m.styles.footer.Width(m.width).Render("↑/↓ select   n new   e edit   s start   x stop   r restart   l logs   g refresh   q quit")
	bodyHeight := max(8, m.height-lipgloss.Height(header)-lipgloss.Height(footer))
	if len(m.profiles) == 0 {
		empty := m.styles.panelActive.Width(min(66, m.width-4)).Render(m.styles.title.Render("No servers configured") + "\n\n" + m.styles.muted.Render("Press n to create a validated Gryt server profile and Docker deployment."))
		body := lipgloss.Place(m.width, bodyHeight, lipgloss.Center, lipgloss.Center, empty)
		return header + "\n" + body + "\n" + footer
	}
	railWidth := min(30, max(22, m.width/3))
	detailWidth := max(36, m.width-railWidth-3)
	var rows []string
	for i, profile := range m.profiles {
		state := m.states[profile.ID]
		glyph := m.styles.muted.Render("○")
		if state == gruntime.StateRunning {
			glyph = m.styles.success.Render("●")
		}
		if state == gruntime.StateUnknown {
			glyph = m.styles.warning.Render("◆")
		}
		row := fmt.Sprintf("%s %s", glyph, profile.Name)
		if i == m.selected {
			row = m.styles.accent.Bold(true).Render("› " + row)
		} else {
			row = "  " + row
		}
		rows = append(rows, row)
	}
	rail := m.styles.panel.Width(railWidth).Height(bodyHeight - 2).Render(m.styles.title.Render("Servers") + "\n\n" + strings.Join(rows, "\n"))
	profile, _ := m.selectedProfile()
	state := m.states[profile.ID]
	status := m.styles.muted.Render(string(state))
	if state == gruntime.StateRunning {
		status = m.styles.success.Render("running")
	}
	details := []string{
		m.styles.title.Render(profile.Name) + "  " + status,
		"",
		m.styles.muted.Render("Address") + "\n" + profile.Address(),
		"",
		m.styles.muted.Render("Security") + "\n" + string(profile.Security) + " · " + profile.Security.Description(),
		"",
		m.styles.muted.Render("Voice") + "\n" + fmt.Sprintf("%d seats · %s", profile.VoiceMaxUsers, valueOr(profile.SFUWebSocketURL, "SFU not configured")),
		"",
		m.styles.muted.Render("Storage") + "\n" + profile.StorageBackend,
		"",
		m.styles.warning.Render("Restart required") + "  bind address · port · SFU · identity trust · storage",
		m.styles.success.Render("Live when API is available") + "  name · discovery · join policy · connection gate",
	}
	if m.busy {
		details = append(details, "", m.styles.accent.Render("Working…"))
	}
	if m.notice != "" {
		details = append(details, "", m.styles.success.Render("✓ "+m.notice))
	}
	if m.err != "" {
		details = append(details, "", m.styles.danger.Render("! "+m.err))
	}
	detail := m.styles.panelActive.Width(detailWidth).Height(bodyHeight - 2).Render(strings.Join(details, "\n"))
	body := lipgloss.JoinHorizontal(lipgloss.Top, rail, " ", detail)
	return header + "\n" + body + "\n" + footer
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
