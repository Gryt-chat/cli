package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m Model) viewWizard() string {
	header := m.header("NEW SERVER")
	field := m.wizard.fields[m.wizard.step]
	keys := "enter next/save   shift+tab back   ←/→ choose   esc cancel"
	if len(field.options) > 0 {
		keys = "space tick   ↑/↓ move   enter next/save   shift+tab back   esc cancel"
	}
	footer := m.styles.footer.Width(m.width).Render(keys)
	step, total := m.wizard.progress()
	progress := fmt.Sprintf("Step %d of %d", step, total)
	var control string
	if len(field.options) > 0 {
		lines := make([]string, len(field.options))
		for i, option := range field.options {
			box := "[ ] "
			if option.chosen {
				box = "[x] "
			}
			text := box + option.label
			if i == field.cursor {
				lines[i] = m.styles.accent.Bold(true).Render("› " + text)
			} else {
				lines[i] = m.styles.muted.Render("  " + text)
			}
		}
		control = strings.Join(lines, "\n")
	} else if len(field.choices) > 0 {
		items := make([]string, len(field.choices))
		for i, choice := range field.choices {
			if i == field.choice {
				items[i] = m.styles.accent.Bold(true).Render("[ " + choice + " ]")
			} else {
				items[i] = m.styles.muted.Render("  " + choice + "  ")
			}
		}
		control = strings.Join(items, "   ")
		if field.key == "security" {
			control += "\n\n" + m.styles.muted.Render(securityDescription(field.choices[field.choice]))
		}
		if field.key == "storage" {
			control += "\n\n" + m.styles.muted.Render(storageDescription(field.choices[field.choice]))
		}
	} else {
		control = field.input.View()
	}
	parts := []string{
		m.styles.muted.Render(progress), "", m.styles.title.Render(field.label),
		m.styles.muted.Render(field.helper), "", control,
	}
	if m.wizard.err != "" {
		parts = append(parts, "", m.styles.danger.Render("! "+m.wizard.err))
	}
	panelWidth := min(72, max(44, m.width-8))
	panel := m.styles.panelActive.Width(panelWidth).Render(strings.Join(parts, "\n"))
	bodyHeight := max(8, m.height-lipgloss.Height(header)-lipgloss.Height(footer))
	body := lipgloss.Place(m.width, bodyHeight, lipgloss.Center, lipgloss.Center, panel)
	return header + "\n" + body + "\n" + footer
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
	header := m.header("LOGS")
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
	panel := m.styles.panelActive.Width(max(40, m.width-4)).Height(maxLines).Render(strings.Join(lines, "\n"))
	return header + "\n" + panel + "\n" + footer
}
