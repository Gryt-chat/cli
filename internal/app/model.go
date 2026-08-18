package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"net/http"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Gryt-chat/cli/internal/config"
	gruntime "github.com/Gryt-chat/cli/internal/runtime"
	"github.com/Gryt-chat/cli/internal/updater"
)

type mode int

const (
	modeDashboard mode = iota
	modeWizard
	modeLogs
	modeDetail
)

type profilesLoaded struct {
	profiles []config.Profile
	err      error
}
type statusesLoaded struct {
	states map[string]gruntime.State
	// Whether the SFU every server here shares is answering. A server can be
	// running perfectly while voice is dead because the shared project is not
	// up, and nothing on the dashboard used to say so.
	sharedUp bool
}
type operationDone struct {
	message string
	err     error
}

// A refresh of logs already on screen, as opposed to opening the view.
type logsFollowed struct{ content string }

type logsLoaded struct {
	profile string
	content string
	err     error
}

// tick drives the dashboard's own refresh. Without it the status was whatever
// it had been when you last pressed g, so a server that fell over looked fine
// until you thought to ask.
type tick time.Time

const (
	// Slow enough that a machine with several servers is not doing constant
	// health checks, quick enough that the panel is not lying for long.
	statusInterval = 5 * time.Second
	// Logs are a page somebody is actively watching, so they follow faster.
	logInterval = 2 * time.Second
)

func tickAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tick(t) })
}

// A newer release exists. Sent once at startup and never again, so the dashboard
// does not poll while it sits open.
type updateFound struct{ tag string }
type updateApplied struct {
	tag string
	err error
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
	version  string
	// Set when a newer release exists, which is what turns on the u key.
	updateTag string
	updating  bool
	sharedUp  bool
	// True while a refresh is in flight, so ticks do not stack up behind a
	// slow health check.
	refreshing bool
}

func New(store *config.Store, manager gruntime.Manager, version string) Model {
	return Model{store: store, runtime: manager, styles: newStyles(), states: map[string]gruntime.State{}, width: 100, height: 30, version: version}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadProfiles(), m.checkForUpdate(), tickAfter(statusInterval))
}

// checkForUpdate asks GitHub which release is newest, once, at startup.
//
// It fails silently. Somebody managing a server on a machine with no route to
// the internet should see their servers, not an error about a version check
// they did not ask for.
func (m Model) checkForUpdate() tea.Cmd {
	current := m.version
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		release, err := updater.Check(ctx, http.DefaultClient)
		if err != nil || !updater.Newer(current, release.Tag) {
			return nil
		}
		return updateFound{tag: release.Tag}
	}
}

func (m Model) applyUpdate() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		release, err := updater.Check(ctx, http.DefaultClient)
		if err != nil {
			return updateApplied{err: err}
		}
		path, err := updater.Path()
		if err != nil {
			return updateApplied{err: err}
		}
		if err := updater.Apply(ctx, http.DefaultClient, release, path); err != nil {
			return updateApplied{err: err}
		}
		return updateApplied{tag: release.Tag}
	}
}

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
		return statusesLoaded{states: states, sharedUp: sharedIsUp(ctx)}
	}
}

// sharedIsUp asks the SFU whether it is there. It publishes its signalling
// port, so this needs nothing from Docker.
func sharedIsUp(ctx context.Context) bool {
	client := http.Client{Timeout: 900 * time.Millisecond}
	url := "http://127.0.0.1:" + strconv.Itoa(config.SFUPort) + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode >= 200 && res.StatusCode < 300
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
		// Ask before doing rather than after failing. Available was declared on
		// the Manager interface from the start and never called, so the
		// operator met a raw compose error instead of "Docker is not running".
		if err := m.runtime.Available(ctx); err != nil {
			return operationDone{err: err}
		}
		var err error
		switch action {
		case "start":
			// The SFU lives in its own project shared by every server here, so
			// it has to exist and be running before a server that expects to
			// reach it comes up.
			if _, err := m.store.WriteSharedCompose(); err != nil {
				return operationDone{err: err}
			}
			if err := m.runtime.EnsureShared(ctx, m.store.SharedDir()); err != nil {
				return operationDone{err: fmt.Errorf("starting the shared SFU: %w", err)}
			}
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

// followLogs is loadLogs without the side effects of opening the view: it
// refreshes what is already on screen and stays quiet when it cannot, so a
// container that goes away mid-follow does not replace the logs with an error.
func (m Model) followLogs(profile config.Profile) tea.Cmd {
	dir := m.store.ServerDir(profile.ID)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		logs, err := m.runtime.Logs(ctx, profile, dir, 120)
		if err != nil {
			return logsFollowed{}
		}
		return logsFollowed{content: logs}
	}
}

func (m Model) loadLogs(profile config.Profile) tea.Cmd {
	dir := m.store.ServerDir(profile.ID)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		if err := m.runtime.Available(ctx); err != nil {
			return logsLoaded{profile: profile.Name, err: err}
		}
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
	case tick:
		// One refresh in flight at a time. A slow health check must not stack
		// up behind itself, and an operation already reloads when it finishes.
		if m.busy || m.refreshing {
			return m, tickAfter(statusInterval)
		}
		if m.mode == modeLogs {
			if profile, ok := m.selectedProfile(); ok {
				m.refreshing = true
				return m, tea.Batch(m.followLogs(profile), tickAfter(logInterval))
			}
			return m, tickAfter(logInterval)
		}
		if m.mode == modeDashboard && len(m.profiles) > 0 {
			m.refreshing = true
			return m, tea.Batch(m.loadStatuses(), tickAfter(statusInterval))
		}
		return m, tickAfter(statusInterval)

	case statusesLoaded:
		m.refreshing = false
		m.states = msg.states
		m.sharedUp = msg.sharedUp
	case operationDone:
		m.busy = false
		if m.mode == modeWizard {
			if msg.err != nil {
				// Back to the step, with the reason, rather than to a
				// dashboard that cannot show what went wrong.
				m.wizard.err = msg.err.Error()
				return m, nil
			}
			m.mode = modeDashboard
		}
		if msg.err != nil {
			m.err, m.notice = msg.err.Error(), ""
		} else {
			m.notice, m.err = msg.message, ""
		}
		return m, tea.Batch(m.loadProfiles(), tea.Tick(800*time.Millisecond, func(time.Time) tea.Msg { return statusesLoaded{states: m.states} }))
	case logsFollowed:
		m.refreshing = false
		if msg.content != "" {
			m.logs = msg.content
		}

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

	switch msg := msg.(type) {
	case updateFound:
		m.updateTag = msg.tag
	case updateApplied:
		m.updating = false
		if msg.err != nil {
			m.err, m.notice = msg.err.Error(), ""
		} else {
			m.updateTag = ""
			m.notice, m.err = "Updated to "+msg.tag+". Restart gryt to run it.", ""
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
					// Stays in the wizard while it writes. Leaving immediately
					// meant the saving state had nowhere to appear, and a
					// failed save dropped you on the dashboard with an error
					// about a form you could no longer see.
					m.busy, m.wizard.err = true, ""
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
	if m.mode == modeDetail && isKey && (key.String() == "esc" || key.String() == "q") {
		m.mode = modeDashboard
		return m, nil
	}
	// Anything else falls through to the same key handling the table uses, so
	// the detail view can act on the server it is showing. It used to swallow
	// every key but esc, while its own footer listed s, x, r and l — naming
	// keys that did nothing on the one screen dedicated to that server.
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
		m.mode, m.wizard, m.notice, m.err = modeWizard, newWizard(config.PortsInUse(m.profiles)), "", ""
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
	case "enter":
		if hasProfile {
			m.mode = modeDetail
		}
		return m, nil
	case "l":
		if hasProfile {
			m.busy = true
			return m, m.loadLogs(profile)
		}
	case "u":
		if m.updateTag == "" || m.updating {
			return m, nil
		}
		m.updating, m.notice, m.err = true, "", ""
		return m, m.applyUpdate()
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
	case modeDetail:
		content = m.viewDetail()
	default:
		content = m.viewDashboard()
	}
	view := tea.NewView(m.styles.base.Width(m.width).Height(m.height).Render(content))
	view.AltScreen = true
	view.WindowTitle = "Gryt server manager"
	return view
}

// joinAddresses turns the bind address into the addresses somebody can
// actually reach this server on.
func (m Model) joinAddresses(profile config.Profile) []string {
	port := strconv.Itoa(profile.Port)
	if profile.Host != "0.0.0.0" {
		return []string{profile.Host + ":" + port}
	}
	// Bound to everything, so every address of this machine reaches it.
	// Reachable addresses first and loopback last: the panel above this says
	// "give people this address", and leading with 127.0.0.1 answers that with
	// the one address nobody else can use.
	var lines []string
	for _, address := range config.LocalAddresses() {
		lines = append(lines, address.IP+":"+port+m.styles.muted.Render("   ("+address.Label+")"))
	}
	return append(lines, "127.0.0.1:"+port+m.styles.muted.Render("   (this machine only)"))
}

// voiceLine says whether voice will work, which is what somebody wants to
// know. The list of endpoints underneath it is detail.
func (m Model) voiceLine(profile config.Profile) string {
	seats := "unlimited"
	if profile.VoiceMaxUsers > 0 {
		seats = strconv.Itoa(profile.VoiceMaxUsers) + " seats"
	}
	routes := len(strings.Split(profile.SFUWebSocketURL, ","))
	if profile.SFUWebSocketURL == "" {
		return m.styles.warning.Render("no route configured") + " · " + seats
	}
	if !m.sharedUp {
		return m.styles.warning.Render("shared voice server is not running") + " · " + seats
	}
	return m.styles.success.Render("ready") + fmt.Sprintf(" · %s · %d route(s)", seats, routes)
}

func storageLine(backend string) string {
	switch backend {
	case "filesystem":
		return "in this server's own folder"
	case "s3":
		return "in the S3 service you configured"
	default:
		return "handled on this machine, with thumbnails"
	}
}

func (m Model) header(section string) string {
	left := m.styles.brand.Render("gryt") + m.styles.muted.Render("  server manager")
	// Rendered as given. The dashboard's summary carries its own colour, and
	// wrapping it in muted here silently flattened it.
	right := section
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	return m.styles.header.Width(m.width).Render(left + strings.Repeat(" ", gap) + right)
}

// viewDashboard is a console table: every server and its live state on one
// screen, the way k9s or docker ps present a fleet.
//
// It replaces a two-pane rail-and-panel where twelve identically-styled labels
// sat inside borders running at 1.45:1 against their own background. Nothing
// was primary, so nothing could be found. Here the row is the unit, the
// columns are the facts, and weight separates the selected row from the rest.
func (m Model) viewDashboard() string {
	noun := "servers"
	if len(m.profiles) == 1 {
		noun = "server"
	}
	summary := fmt.Sprintf("%d %s", len(m.profiles), noun)

	// Only worth reporting when something is running: a fleet that is simply
	// stopped does not have a voice problem.
	running := 0
	for _, profile := range m.profiles {
		if m.states[profile.ID] == gruntime.StateRunning {
			running++
		}
	}
	if running > 0 {
		if m.sharedUp {
			summary += " · " + m.styles.success.Render("voice ready")
		} else {
			summary += " · " + m.styles.warning.Render("voice server down")
		}
	}
	head := m.header(summary)

	keys := "↑/↓ select   enter details   s start   x stop   r restart   l logs   e edit   n new   q quit"
	if m.updateTag != "" {
		keys = "u update   " + keys
	}
	footer := m.styles.footer.Width(m.width).Render(" " + keys)

	if len(m.profiles) == 0 {
		empty := m.styles.title.Render("No servers yet") + "\n\n" +
			m.styles.muted.Render("Press n to make one. It runs on this machine and takes about a minute.")
		body := lipgloss.Place(m.width, max(4, m.height-2), lipgloss.Center, lipgloss.Center, empty)
		return head + "\n" + body + "\n" + footer
	}

	// Widths are fixed for the facts and flexible for the name, so the columns
	// stay put as servers come and go rather than jumping about on each poll.
	const (
		statusW  = 10
		addressW = 22
		voiceW   = 8
		uploadsW = 8
	)
	// Capped, not greedy. Giving the name every spare column pushed the facts
	// to the far right of a wide terminal, so the eye had to travel the whole
	// width to pair a server with its state.
	nameW := max(12, min(28, m.width-(statusW+addressW+voiceW+uploadsW)-8))

	lines := []string{m.styles.column.Render("  " +
		pad("NAME", nameW) + pad("STATUS", statusW) + pad("ADDRESS", addressW) +
		pad("VOICE", voiceW) + pad("UPLOADS", uploadsW)), ""}

	for i, profile := range m.profiles {
		glyph, word, tone := m.stateOf(profile)
		row := pad(truncate(profile.Name, nameW-1), nameW) +
			pad(glyph+" "+word, statusW) +
			pad(truncate(m.primaryAddress(profile), addressW-1), addressW) +
			pad(m.voiceCell(profile), voiceW) +
			pad(uploadsCell(profile.StorageBackend), uploadsW)

		switch {
		case i == m.selected:
			// Weight, not a box. The selected row is the only bold thing on
			// screen, so the eye lands on it without anything being drawn.
			lines = append(lines, m.styles.strong.Render("› "+row))
		default:
			lines = append(lines, tone.Render("  ")+m.styles.muted.Render(row))
		}
	}

	if m.updating {
		lines = append(lines, "", m.styles.muted.Render(" Updating…"))
	} else if m.updateTag != "" {
		lines = append(lines, "", m.styles.accent.Render(" "+m.updateTag+" is available. Press u to update."))
	}
	if m.busy {
		lines = append(lines, "", m.styles.accent.Render(" Working…"))
	}
	if m.notice != "" {
		lines = append(lines, "", m.styles.success.Render(" ✓ "+m.notice))
	}
	if m.err != "" {
		lines = append(lines, "", m.styles.danger.Render(" ! "+m.err))
	}

	body := lipgloss.NewStyle().Height(max(4, m.height-2)).Render(strings.Join(lines, "\n"))
	return head + "\n" + body + "\n" + footer
}

// viewDetail is one server, with the facts ranked.
//
// The panel it replaces rendered twelve fields as identical muted labels, so
// the address you hand somebody had exactly the weight of the join policy and
// the eye had nowhere to land. Here one fact is the page and the rest is a
// single line under it.
func (m Model) viewDetail() string {
	profile, ok := m.selectedProfile()
	if !ok {
		return m.viewDashboard()
	}
	glyph, word, tone := m.stateOf(profile)
	head := m.header(tone.Render(glyph + " " + word))
	keys := " esc back   s start   x stop   r restart   l logs   e edit"
	if m.updateTag != "" {
		keys = " u update  " + keys
	}
	footer := m.styles.footer.Width(m.width).Render(keys)

	addresses := m.joinAddresses(profile)
	lines := []string{
		"",
		"  " + m.styles.title.Render(profile.Name),
		"",
		"  " + m.styles.muted.Render("Give people this address"),
	}
	for i, address := range addresses {
		// The first is the one to read; the rest are alternatives.
		if i == 0 {
			lines = append(lines, "  "+m.styles.strong.Render(address))
			continue
		}
		lines = append(lines, "  "+m.styles.muted.Render(address))
	}

	lines = append(lines,
		"",
		"  "+m.styles.muted.Render(strings.Join([]string{
			"voice " + m.voiceCell(profile),
			"uploads " + uploadsCell(profile.StorageBackend),
			profile.Security.Description(),
		}, "  ·  ")),
	)

	if m.err != "" {
		lines = append(lines, "", "  "+m.styles.danger.Render("! "+m.err))
	}

	body := lipgloss.NewStyle().Height(max(4, m.height-2)).Render(strings.Join(lines, "\n"))
	return head + "\n" + body + "\n" + footer
}

// stateOf pairs every status with a glyph as well as a colour, so the fact
// survives a colour-blind reader and a monochrome terminal.
func (m Model) stateOf(profile config.Profile) (glyph, word string, tone lipgloss.Style) {
	switch m.states[profile.ID] {
	case gruntime.StateRunning:
		return "●", "running", m.styles.success
	case gruntime.StateUnknown:
		return "◆", "unknown", m.styles.warning
	default:
		return "○", "stopped", m.styles.muted
	}
}

// primaryAddress is the one to read at a glance: the address somebody else can
// reach, in preference to loopback, and never the wildcard bind.
func (m Model) primaryAddress(profile config.Profile) string {
	port := strconv.Itoa(profile.Port)
	if profile.Host != "0.0.0.0" {
		return profile.Host + ":" + port
	}
	if addresses := config.LocalAddresses(); len(addresses) > 0 {
		return addresses[0].IP + ":" + port
	}
	return "127.0.0.1:" + port
}

func (m Model) voiceCell(profile config.Profile) string {
	// A stopped server has no voice to report on. Saying "down" there reads as
	// a fault when it is simply not running.
	if m.states[profile.ID] != gruntime.StateRunning {
		return "—"
	}
	if profile.SFUWebSocketURL == "" {
		return "none"
	}
	if !m.sharedUp {
		return "down"
	}
	return "ready"
}

func uploadsCell(backend string) string {
	switch backend {
	case "filesystem":
		return "folder"
	case "s3":
		return "s3"
	default:
		return "here"
	}
}

// pad fills a cell to width with spaces. The cells are built plain and styled
// afterwards, because padding a string that already carries escape codes
// measures the codes and the columns drift.
func pad(text string, width int) string {
	for lipgloss.Width(text) < width {
		text += " "
	}
	return text + " "
}

func truncate(text string, width int) string {
	if lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	if width < 2 || len(runes) < 2 {
		return text
	}
	return string(runes[:width-1]) + "…"
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
