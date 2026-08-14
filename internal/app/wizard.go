package app

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/Gryt-chat/cli/internal/config"
)

type wizardField struct {
	key, label, helper string
	input              textinput.Model
	choices            []string
	choice             int
}

type wizard struct {
	fields   []wizardField
	step     int
	err      string
	original *config.Profile
}

func inputField(key, label, helper, value, placeholder string) wizardField {
	input := textinput.New()
	inputStyles := textinput.DefaultDarkStyles()
	inputStyles.Focused.Text = inputStyles.Focused.Text.Foreground(cobalt.text.value())
	inputStyles.Focused.Placeholder = inputStyles.Focused.Placeholder.Foreground(cobalt.muted.value())
	inputStyles.Blurred.Text = inputStyles.Blurred.Text.Foreground(cobalt.muted.value())
	inputStyles.Blurred.Placeholder = inputStyles.Blurred.Placeholder.Foreground(cobalt.rule.value())
	inputStyles.Cursor.Color = cobalt.accent.value()
	input.SetStyles(inputStyles)
	input.Prompt = ""
	input.Placeholder = placeholder
	input.SetValue(value)
	input.SetWidth(48)
	return wizardField{key: key, label: label, helper: helper, input: input}
}

func selectField(key, label, helper string, choices []string, current int) wizardField {
	return wizardField{key: key, label: label, helper: helper, choices: choices, choice: current}
}

func newWizard() wizard {
	levels := config.SecurityLevels()
	security := make([]string, len(levels))
	for i, level := range levels {
		security[i] = string(level)
	}
	w := wizard{fields: []wizardField{
		inputField("name", "Server name", "Shown to people who connect.", "", "My Gryt Server"),
		inputField("host", "Bind address", "127.0.0.1 is local-only; use 0.0.0.0 behind a firewall or proxy.", "127.0.0.1", "127.0.0.1"),
		inputField("port", "Port", "TCP port exposed by Docker and used by clients.", "5000", "5000"),
		selectField("security", "Security level", "Strict hides discovery; Community permits local identities.", security, 1),
		inputField("voice", "Voice seats", "Maximum concurrent voice users; 0 means unlimited.", "20", "20"),
		inputField("proxy", "Trusted proxy hops", "Set to 1 for one reverse proxy or tunnel; otherwise leave 0.", "0", "0"),
		inputField("sfu", "SFU WebSocket URL", "Optional for now. Voice remains unavailable until an SFU is configured.", "", "wss://…"),
		selectField("storage", "Storage backend", "Filesystem is simplest; S3 needs credentials in Advanced env.", []string{"filesystem", "s3"}, 0),
	}}
	w.focus()
	return w
}

func wizardFromProfile(profile config.Profile) wizard {
	w := newWizard()
	w.original = &profile
	values := map[string]string{
		"name": profile.Name, "host": profile.Host, "port": strconv.Itoa(profile.Port),
		"voice": strconv.Itoa(profile.VoiceMaxUsers), "proxy": strconv.Itoa(profile.TrustedProxyHops),
		"sfu": profile.SFUWebSocketURL,
	}
	for i := range w.fields {
		field := &w.fields[i]
		if value, ok := values[field.key]; ok {
			field.input.SetValue(value)
		}
		for choice, value := range field.choices {
			if (field.key == "security" && value == string(profile.Security)) || (field.key == "storage" && value == profile.StorageBackend) {
				field.choice = choice
			}
		}
	}
	w.focus()
	return w
}

func (w *wizard) focus() tea.Cmd {
	for i := range w.fields {
		w.fields[i].input.Blur()
	}
	if len(w.fields[w.step].choices) == 0 {
		return w.fields[w.step].input.Focus()
	}
	return nil
}

func (w *wizard) update(msg tea.Msg) tea.Cmd {
	field := &w.fields[w.step]
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "left", "h":
			if len(field.choices) > 0 {
				field.choice = (field.choice - 1 + len(field.choices)) % len(field.choices)
				return nil
			}
		case "right", "l", "space":
			if len(field.choices) > 0 {
				field.choice = (field.choice + 1) % len(field.choices)
				return nil
			}
		}
	}
	if len(field.choices) == 0 {
		updated, cmd := field.input.Update(msg)
		field.input = updated
		return cmd
	}
	return nil
}

func (w *wizard) next() tea.Cmd {
	if err := w.validateStep(); err != nil {
		w.err = err.Error()
		return nil
	}
	w.err = ""
	if w.step < len(w.fields)-1 {
		w.step++
		return w.focus()
	}
	return nil
}

func (w *wizard) previous() tea.Cmd {
	w.err = ""
	if w.step > 0 {
		w.step--
		return w.focus()
	}
	return nil
}

func (w wizard) complete() bool { return w.step == len(w.fields)-1 && w.validateStep() == nil }

func (w wizard) validateStep() error {
	f := w.fields[w.step]
	value := strings.TrimSpace(f.input.Value())
	switch f.key {
	case "name":
		if value == "" {
			return fmt.Errorf("enter a server name")
		}
	case "port":
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
	case "voice":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("voice seats must be zero or greater")
		}
	case "proxy":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 16 {
			return fmt.Errorf("proxy hops must be between 0 and 16")
		}
	}
	return nil
}

func (w wizard) profile() (config.Profile, error) {
	values := map[string]string{}
	for _, field := range w.fields {
		if len(field.choices) > 0 {
			values[field.key] = field.choices[field.choice]
		} else {
			values[field.key] = strings.TrimSpace(field.input.Value())
		}
	}
	profile := config.NewProfile(values["name"])
	if w.original != nil {
		profile.ID = w.original.ID
		profile.CreatedAt = w.original.CreatedAt
		profile.ExtraEnv = w.original.ExtraEnv
	}
	profile.Host = values["host"]
	profile.Port, _ = strconv.Atoi(values["port"])
	profile.Security = config.SecurityLevel(values["security"])
	profile.VoiceMaxUsers, _ = strconv.Atoi(values["voice"])
	profile.TrustedProxyHops, _ = strconv.Atoi(values["proxy"])
	profile.SFUWebSocketURL = values["sfu"]
	profile.StorageBackend = values["storage"]
	return profile, profile.Validate()
}
