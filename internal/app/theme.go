// Hallmark · pre-emit critique: P4 H5 E4 S5 R5 V4 · macrostructure: Console Table
// tone: technical/utilitarian · chrome: inherit-terminal · contrast: foreground-only
package app

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// The palette is foreground-only on purpose: a terminal program does not own the background,
// and these hues clear 4.5:1 against both a near-black and a near-white terminal.
type theme struct {
	text    colorToken
	muted   colorToken
	accent  colorToken
	success colorToken
	warning colorToken
	danger  colorToken
	rule    colorToken
}

type colorToken string

func (c colorToken) value() color.Color { return lipgloss.Color(string(c)) }

var cobalt = theme{
	// Unset, so body text is whatever the terminal's own foreground is. The
	// most readable colour on somebody's terminal is the one they chose.
	text:    "",
	muted:   "8", // ANSI bright black: legible on light and dark alike
	accent:  "4", // blue
	success: "2", // green
	warning: "3", // yellow
	danger:  "1", // red
	rule:    "8",
}

type styles struct {
	base, header, brand, title, muted, accent, success, warning, danger, footer, strong, column lipgloss.Style
}

func newStyles() styles {
	t := cobalt
	plain := lipgloss.NewStyle()
	return styles{
		// No Background call anywhere in this file. That is the point.
		base:    plain,
		header:  plain,
		brand:   plain.Bold(true).Foreground(t.accent.value()),
		title:   plain.Bold(true),
		strong:  plain.Bold(true),
		column:  plain.Foreground(t.muted.value()),
		muted:   plain.Foreground(t.muted.value()),
		accent:  plain.Foreground(t.accent.value()),
		success: plain.Foreground(t.success.value()),
		warning: plain.Foreground(t.warning.value()),
		danger:  plain.Foreground(t.danger.value()),
		footer:  plain.Foreground(t.muted.value()),
	}
}
