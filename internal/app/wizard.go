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
	// Shown only while the field named by whenKey holds whenValue. An empty
	// whenKey means the field is always shown.
	whenKey, whenValue string
	// What an empty field means. Shown as the placeholder and used when the
	// field is left alone, so a default is never text you have to delete.
	fallback string
}

type wizard struct {
	fields   []wizardField
	step     int
	err      string
	original *config.Profile
}

// inputField starts empty with the default shown as the placeholder, rather
// than pre-filled with it.
//
// A pre-filled field puts the cursor at the end, so typing appends: changing
// the port from 5000 to 5001 meant deleting four characters first, and typing
// "uploads" into a bucket field holding "gryt" produced "grytuploads". bubbles
// exposes no selection, so there is no select-on-focus to reach for. Leaving
// the field empty and treating empty as the default gets the same result and
// is less to explain.
func inputField(key, label, helper, fallback, placeholder string) wizardField {
	input := textinput.New()
	inputStyles := textinput.DefaultDarkStyles()
	inputStyles.Focused.Text = inputStyles.Focused.Text.Foreground(cobalt.text.value())
	inputStyles.Focused.Placeholder = inputStyles.Focused.Placeholder.Foreground(cobalt.muted.value())
	inputStyles.Blurred.Text = inputStyles.Blurred.Text.Foreground(cobalt.muted.value())
	inputStyles.Blurred.Placeholder = inputStyles.Blurred.Placeholder.Foreground(cobalt.rule.value())
	inputStyles.Cursor.Color = cobalt.accent.value()
	input.SetStyles(inputStyles)
	input.Prompt = ""
	if placeholder == "" {
		placeholder = fallback
	}
	input.Placeholder = placeholder
	input.SetWidth(48)
	return wizardField{key: key, label: label, helper: helper, input: input, fallback: fallback}
}

func selectField(key, label, helper string, choices []string, current int) wizardField {
	return wizardField{key: key, label: label, helper: helper, choices: choices, choice: current}
}

// onlyWhen makes a field conditional on an earlier answer.
func onlyWhen(field wizardField, key, value string) wizardField {
	field.whenKey, field.whenValue = key, value
	return field
}

// masked hides what is typed. Used for the one field here that is a secret
// rather than merely sensitive: an access key ID identifies an account, but a
// secret access key is the account.
func masked(field wizardField) wizardField {
	field.input.EchoMode = textinput.EchoPassword
	return field
}

func (f wizardField) value() string {
	if len(f.choices) > 0 {
		return f.choices[f.choice]
	}
	if typed := strings.TrimSpace(f.input.Value()); typed != "" {
		return typed
	}
	return f.fallback
}

// newWizard offers the first port nothing else holds, rather than always 5000.
// taken names the ports other servers on this machine already claim.
func newWizard(taken []int) wizard {
	port := strconv.Itoa(config.FreePort(taken))
	levels := config.SecurityLevels()
	security := make([]string, len(levels))
	for i, level := range levels {
		security[i] = string(level)
	}
	w := wizard{fields: []wizardField{
		inputField("name", "Server name", "Shown to people who connect.", "", "My Gryt Server"),
		inputField("host", "Bind address", "0.0.0.0 accepts connections from other machines. Use 127.0.0.1 to keep the server on this one.", "0.0.0.0", "0.0.0.0"),
		inputField("port", "Port", "TCP port exposed by Docker and used by clients. Offered because nothing else on this machine holds it.", port, port),
		selectField("security", "Security level", "Strict hides discovery; Community permits local identities.", security, 1),
		inputField("voice", "Voice seats", "0 means no limit, which is the server's own default. A cap is about your CPU and upload bandwidth, not about ports.", "0", "0"),
		inputField("proxy", "Trusted proxy hops", "Set to 1 for one reverse proxy or tunnel; otherwise leave 0.", "0", "0"),
		inputField("sfu", "Public SFU address", "How clients reach the SFU on this machine. Empty means localhost, which only works for clients on this machine.", "", "ws://192.168.1.20:5005"),
		selectField("storage", "Storage backend", "Filesystem is simplest; choosing S3 asks for its endpoint and credentials next.", []string{"filesystem", "s3"}, 0),

		// Only reachable when the backend is s3. Asking six questions about
		// object storage to somebody who picked the filesystem would be six
		// steps of nothing, and leaving them out entirely is what shipped a
		// backend that could be selected but never configured.
		onlyWhen(inputField("s3endpoint", "S3 endpoint", "Full URL of the S3 API. MinIO on the same host looks like http://minio:9000.", "", "https://s3.eu-central-1.amazonaws.com"), "storage", "s3"),
		onlyWhen(inputField("s3bucket", "Bucket", "Must already exist. Gryt does not create it.", "gryt", "gryt"), "storage", "s3"),
		onlyWhen(inputField("s3region", "Region", "Leave as auto for MinIO and most S3-compatible services.", "auto", "auto"), "storage", "s3"),
		onlyWhen(inputField("s3key", "Access key ID", "The key with read and write access to the bucket.", "", ""), "storage", "s3"),
		onlyWhen(masked(inputField("s3secret", "Secret access key", "Stored in the generated .env, which is readable only by you.", "", "")), "storage", "s3"),
		onlyWhen(selectField("s3path", "Path-style addressing", "MinIO and most self-hosted gateways need this on. AWS does not.", []string{"true", "false"}, 0), "storage", "s3"),
	}}
	w.focus()
	return w
}

func wizardFromProfile(profile config.Profile) wizard {
	// No port search when editing: this server already has one, and offering a
	// different free port would quietly move it.
	w := newWizard(nil)
	w.original = &profile
	values := map[string]string{
		"name": profile.Name, "host": profile.Host, "port": strconv.Itoa(profile.Port),
		"voice": strconv.Itoa(profile.VoiceMaxUsers), "proxy": strconv.Itoa(profile.TrustedProxyHops),
		"sfu": profile.SFUWebSocketURL,
	}
	// Only override a default when the profile actually carries a value, or
	// editing a filesystem server would blank the region and bucket defaults
	// on the way past.
	for key, env := range map[string]string{
		"s3endpoint": "S3_ENDPOINT",
		"s3bucket":   "S3_BUCKET",
		"s3region":   "S3_REGION",
		"s3key":      "S3_ACCESS_KEY_ID",
		"s3secret":   "S3_SECRET_ACCESS_KEY",
	} {
		if value := profile.ExtraEnv[env]; value != "" {
			values[key] = value
		}
	}
	for i := range w.fields {
		field := &w.fields[i]
		if value, ok := values[field.key]; ok {
			field.input.SetValue(value)
		}
		for choice, value := range field.choices {
			if (field.key == "security" && value == string(profile.Security)) ||
				(field.key == "storage" && value == profile.StorageBackend) ||
				(field.key == "s3path" && value == profile.ExtraEnv["S3_FORCE_PATH_STYLE"]) {
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

// s3EnvKeys are the variables the wizard owns when the backend is s3. They are
// listed once so that switching back to the filesystem can clear exactly these
// and leave anything an operator added by hand alone.
var s3EnvKeys = []string{
	"S3_ENDPOINT",
	"S3_BUCKET",
	"S3_REGION",
	"S3_ACCESS_KEY_ID",
	"S3_SECRET_ACCESS_KEY",
	"S3_FORCE_PATH_STYLE",
}

func (w wizard) shown(i int) bool {
	field := w.fields[i]
	if field.whenKey == "" {
		return true
	}
	for _, other := range w.fields {
		if other.key == field.whenKey {
			return other.value() == field.whenValue
		}
	}
	return false
}

func (w wizard) visible() []int {
	steps := make([]int, 0, len(w.fields))
	for i := range w.fields {
		if w.shown(i) {
			steps = append(steps, i)
		}
	}
	return steps
}

// Position of the current step among the visible ones, and how many there are.
// The count moves as the storage answer changes, which is honest: it is the
// number of questions actually left.
func (w wizard) progress() (int, int) {
	steps := w.visible()
	for n, i := range steps {
		if i == w.step {
			return n + 1, len(steps)
		}
	}
	return 1, len(steps)
}

func (w *wizard) next() tea.Cmd {
	if err := w.validateStep(); err != nil {
		w.err = err.Error()
		return nil
	}
	w.err = ""
	steps := w.visible()
	for n, i := range steps {
		if i == w.step && n+1 < len(steps) {
			w.step = steps[n+1]
			return w.focus()
		}
	}
	return nil
}

func (w *wizard) previous() tea.Cmd {
	w.err = ""
	steps := w.visible()
	for n, i := range steps {
		if i == w.step && n > 0 {
			w.step = steps[n-1]
			return w.focus()
		}
	}
	return nil
}

// onLastStep reports whether enter should save rather than advance.
//
// The dashboard used to work this out for itself with
// `step == len(fields)-1`, which was the same answer while every field was
// always shown. Once fields became conditional the two definitions disagreed:
// a filesystem server sits on step 8 of 8 while the last field in the slice is
// the sixth S3 one, so enter fell through to next(), which had nowhere to go,
// and the wizard could not be saved at all.
func (w wizard) onLastStep() bool {
	steps := w.visible()
	return len(steps) > 0 && w.step == steps[len(steps)-1]
}

func (w wizard) complete() bool { return w.onLastStep() && w.validateStep() == nil }

func (w wizard) validateStep() error {
	f := w.fields[w.step]
	value := f.value()
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
	case "s3endpoint":
		if value == "" {
			return fmt.Errorf("enter the S3 endpoint URL")
		}
		if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			return fmt.Errorf("endpoint must start with http:// or https://")
		}
	case "s3bucket":
		if value == "" {
			return fmt.Errorf("enter the bucket name")
		}
	case "s3region":
		if value == "" {
			return fmt.Errorf("enter a region, or auto")
		}
	case "s3key":
		if value == "" {
			return fmt.Errorf("enter the access key ID")
		}
	case "s3secret":
		if value == "" {
			return fmt.Errorf("enter the secret access key")
		}
	}
	return nil
}

func (w wizard) profile() (config.Profile, error) {
	values := map[string]string{}
	for _, field := range w.fields {
		values[field.key] = field.value()
	}
	profile := config.NewProfile(values["name"])

	// The S3 answers are environment variables rather than profile fields, so
	// they travel in ExtraEnv next to anything set outside the wizard. Those
	// other keys are preserved; the six below are rewritten from the answers,
	// and cleared when the backend is not s3 so that credentials do not sit in
	// the file for a backend nothing is using.
	extra := map[string]string{}
	if w.original != nil {
		profile.ID = w.original.ID
		profile.CreatedAt = w.original.CreatedAt
		for key, value := range w.original.ExtraEnv {
			extra[key] = value
		}
	}
	for _, key := range s3EnvKeys {
		delete(extra, key)
	}
	if values["storage"] == "s3" {
		extra["S3_ENDPOINT"] = values["s3endpoint"]
		extra["S3_BUCKET"] = values["s3bucket"]
		extra["S3_REGION"] = values["s3region"]
		extra["S3_ACCESS_KEY_ID"] = values["s3key"]
		extra["S3_SECRET_ACCESS_KEY"] = values["s3secret"]
		extra["S3_FORCE_PATH_STYLE"] = values["s3path"]
	}
	profile.ExtraEnv = extra
	profile.Host = values["host"]
	profile.Port, _ = strconv.Atoi(values["port"])
	profile.Security = config.SecurityLevel(values["security"])
	profile.VoiceMaxUsers, _ = strconv.Atoi(values["voice"])
	profile.TrustedProxyHops, _ = strconv.Atoi(values["proxy"])
	profile.SFUWebSocketURL = values["sfu"]
	profile.StorageBackend = values["storage"]
	return profile, profile.Validate()
}
