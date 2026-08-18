package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// viewWizard is one question at a time, in the same language as the console
// table: left-aligned, hierarchy carried by weight, nothing drawn.
//
// It used to render inside panelActive, which the dashboard redesign turned
// into a no-op style. That left every step as unframed text floating after
// eight blank lines, with no visual relationship between the counter, the
// question, the help and the field. This does not restore the box; it gives
// the step a structure that does not depend on one.
func (m Model) viewWizard() string {
	field := m.wizard.fields[m.wizard.step]
	step, total := m.wizard.progress()

	head := m.header(m.styles.muted.Render("NEW SERVER"))

	keys := "enter next   shift+tab back   ←/→ choose   esc cancel"
	switch {
	case len(field.options) > 0:
		keys = "space tick   ↑/↓ move   enter next   shift+tab back   esc cancel"
	case m.wizard.complete():
		keys = "enter save   shift+tab back   esc cancel"
	}
	footer := m.styles.footer.Width(m.width).Render(" " + keys)

	lines := []string{
		"",
		"  " + m.styles.muted.Render(fmt.Sprintf("Step %d of %d", step, total)) + "   " + m.progressDots(step, total),
		"",
		"  " + m.styles.title.Render(field.label),
	}
	if field.helper != "" {
		lines = append(lines, "  "+m.styles.muted.Render(wrapText(field.helper, max(30, min(72, m.width-4)))))
	}
	lines = append(lines, "", "  "+m.control(field))

	// The four states a full-screen keyboard-driven step actually has.
	// Saving is shown here, on the step that triggered it, rather than only as
	// a notice on the dashboard after the wizard has already closed.
	switch {
	case m.busy:
		lines = append(lines, "", "  "+m.styles.accent.Render("Saving…"))
	case m.wizard.err != "":
		lines = append(lines, "", "  "+m.styles.danger.Render("! "+m.wizard.err))
	}

	body := lipgloss.NewStyle().Height(max(4, m.height-2)).Render(strings.Join(lines, "\n"))
	return head + "\n" + body + "\n" + footer
}

// progressDots shows where you are without counting. Filled for done, hollow
// for remaining; the total moves when an answer adds or removes steps, which
// is honest about a wizard whose length depends on what you pick.
func (m Model) progressDots(step, total int) string {
	if total > 20 {
		return ""
	}
	done := strings.Repeat("●", step)
	rest := strings.Repeat("○", max(0, total-step))
	return m.styles.accent.Render(done) + m.styles.muted.Render(rest)
}

// control renders whichever kind of answer this step takes.
func (m Model) control(field wizardField) string {
	switch {
	case len(field.options) > 0:
		lines := make([]string, len(field.options))
		for i, option := range field.options {
			box := "[ ] "
			if option.chosen {
				box = "[x] "
			}
			text := box + option.label
			if i == field.cursor {
				lines[i] = m.styles.strong.Render("› " + text)
			} else {
				lines[i] = m.styles.muted.Render("  " + text)
			}
		}
		return strings.Join(lines, "\n  ")

	case len(field.choices) > 0:
		items := make([]string, len(field.choices))
		for i, choice := range field.choices {
			label := choice
			if choice == field.recommended {
				label += " (recommended)"
			}
			if i == field.choice {
				items[i] = m.styles.strong.Render("[ " + label + " ]")
			} else {
				items[i] = m.styles.muted.Render("  " + label + "  ")
			}
		}
		control := strings.Join(items, "   ")
		switch field.key {
		case "security":
			control += "\n\n  " + m.styles.muted.Render(securityDescription(field.choices[field.choice]))
		case "storage":
			control += "\n\n  " + m.styles.muted.Render(storageDescription(field.choices[field.choice]))
		}
		return control

	default:
		return field.input.View()
	}
}

// wrapText breaks help text on spaces so a long line does not run off a narrow
// terminal. Continuation lines carry the same indent as the first.
func wrapText(text string, width int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	lines, current := []string{}, words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(current)+1+lipgloss.Width(word) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current += " " + word
	}
	return strings.Join(append(lines, current), "\n  ")
}

func securityDescription(value string) string {
	switch value {
	case "strict":
		return "Accounts only · hidden from discovery · invite-only"
	case "community":
		return "Accounts + local identities · discoverable · invite-only"
	default:
		return "Accounts only · discoverable · invite-only"
	}
}

// Named for what each answer does rather than for how it is built. "Shared"
// and "s3" and "filesystem" are the words the code uses; somebody standing up
// a server for their friends is choosing between "it just works", "keep it
// simple" and "I already pay for storage somewhere".
func storageDescription(value string) string {
	switch value {
	case "filesystem":
		return "Straight into the server's own folder · no extra containers · no thumbnails"
	case "s3":
		return "A storage service you already have · you supply the address and keys"
	default:
		return "Handled for you on this machine · thumbnails and compression included"
	}
}

func (m Model) viewLogs() string {
	header := m.header(m.styles.muted.Render("LOGS"))
	footer := m.styles.footer.Width(m.width).Render("esc back")
	content := m.logs
	if strings.TrimSpace(content) == "" {
		content = "No log output yet."
	}
	lines := strings.Split(content, "\n")
	maxLines := max(4, m.height-7)
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	// Indented rather than boxed, matching the table and the wizard. A frame
	// around log output costs columns that the output itself wants.
	body := lipgloss.NewStyle().Height(maxLines).Render("  " + strings.Join(lines, "\n  "))
	return header + "\n" + body + "\n" + footer
}
