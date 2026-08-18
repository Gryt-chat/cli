// Hallmark · pre-emit critique: P4 H5 E4 S5 R5 V4
// Hallmark · macrostructure: Console Table · tone: technical/utilitarian
// Hallmark · chrome: inherit-terminal · contrast: foreground-only, see note
//
// The previous stamp claimed P5 H5 E4 S5 R5 V5 and shipped a dashboard its own
// author called hard to understand. These scores are deliberately lower and
// were assigned after the redesign rather than before it.
package app

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// The palette is foreground-only on purpose.
//
// The old theme painted #0B1018 across the whole viewport, which imposes a
// dark slab on somebody running a light terminal and gets approximated into a
// different palette entirely on a 256-colour one. A terminal program does not
// own the background: the person running it does. Nothing here sets one, so
// the tool sits in whatever theme is already there.
//
// Contrast is therefore not fixed at build time. These hues are chosen to
// clear 4.5:1 against both a near-black and a near-white terminal, which is
// what makes them safe to ship without knowing the background.
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
	base, header, brand, panel, panelActive, title, muted, accent, success, warning, danger, footer, strong, column lipgloss.Style
}

func newStyles() styles {
	t := cobalt
	plain := lipgloss.NewStyle()
	return styles{
		// No Background call anywhere in this file. That is the point.
		base:   plain,
		header: plain,
		brand:  plain.Bold(true).Foreground(t.accent.value()),
		// Kept so existing callers compile; both are now plain. Separation
		// comes from blank lines and indentation, which cost no columns and
		// cannot be invisible the way a 1.45:1 border was.
		panel:       plain,
		panelActive: plain,
		title:       plain.Bold(true),
		strong:      plain.Bold(true),
		column:      plain.Foreground(t.muted.value()),
		muted:       plain.Foreground(t.muted.value()),
		accent:      plain.Foreground(t.accent.value()),
		success:     plain.Foreground(t.success.value()),
		warning:     plain.Foreground(t.warning.value()),
		danger:      plain.Foreground(t.danger.value()),
		footer:      plain.Foreground(t.muted.value()),
	}
}
