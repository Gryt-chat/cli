// Hallmark · pre-emit critique: P5 H5 E4 S5 R5 V5
// Hallmark · macrostructure: Workbench · tone: technical/utilitarian · anchor hue: cobalt · contrast: pass
package app

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type theme struct {
	canvas  colorToken
	panel   colorToken
	rule    colorToken
	text    colorToken
	muted   colorToken
	accent  colorToken
	success colorToken
	warning colorToken
	danger  colorToken
}

// colorToken keeps raw terminal colour values in one locked token table.
type colorToken string

func (c colorToken) value() color.Color { return lipgloss.Color(string(c)) }

var cobalt = theme{
	canvas:  "#0B1018",
	panel:   "#111A27",
	rule:    "#2A374A",
	text:    "#E8EEF7",
	muted:   "#8FA0B8",
	accent:  "#5E9BFF",
	success: "#61C995",
	warning: "#F1C75B",
	danger:  "#FF7185",
}

type styles struct {
	base, header, brand, panel, panelActive, title, muted, accent, success, warning, danger, footer lipgloss.Style
}

func newStyles() styles {
	t := cobalt
	return styles{
		base:        lipgloss.NewStyle().Foreground(t.text.value()).Background(t.canvas.value()),
		header:      lipgloss.NewStyle().Foreground(t.text.value()).Background(t.panel.value()).Padding(0, 1),
		brand:       lipgloss.NewStyle().Bold(true).Foreground(t.accent.value()),
		panel:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.rule.value()).Padding(1, 2),
		panelActive: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.accent.value()).Padding(1, 2),
		title:       lipgloss.NewStyle().Bold(true).Foreground(t.text.value()),
		muted:       lipgloss.NewStyle().Foreground(t.muted.value()),
		accent:      lipgloss.NewStyle().Foreground(t.accent.value()),
		success:     lipgloss.NewStyle().Foreground(t.success.value()),
		warning:     lipgloss.NewStyle().Foreground(t.warning.value()),
		danger:      lipgloss.NewStyle().Foreground(t.danger.value()),
		footer:      lipgloss.NewStyle().Foreground(t.muted.value()).Background(t.panel.value()).Padding(0, 1),
	}
}
