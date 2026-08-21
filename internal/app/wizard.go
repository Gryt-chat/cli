package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/Gryt-chat/cli/internal/config"
)

// A choice somebody can tick in a multi-select. The label is what they read,
// the value is what ends up in the configuration.
type multiOption struct {
	label, value string
	chosen       bool
}

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
	// Which of choices is the one to pick when you have no reason to prefer
	// another. Empty when the answer genuinely depends on the situation, so
	// that the badge means something wherever it appears.
	recommended string
	// Set for a tick-list. Some questions have more than one right answer at
	// the same time: a server reachable over a LAN and over the internet is
	// reachable over both, and the client picks whichever is faster.
	options []multiOption
	cursor  int
}

type wizard struct {
	// Chosen when the wizard opens, so a server created here has a management
	// port before anything tries to manage it.
	adminPort int
	fields    []wizardField
	step      int
	err       string
	original  *config.Profile
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

// reachField asks where people will connect from, offering this machine's own
// addresses rather than an empty box.
//
// It replaces a free-text "SFU WebSocket URL" with a `wss://…` placeholder,
// which nobody who did not already know the answer could fill in, and getting
// it wrong is how voice silently fails. The answers become SFU_PUBLIC_HOST,
// which takes a comma-separated list: the client pings each and uses whichever
// answers fastest, so ticking both a LAN address and a public one is a
// sensible thing to do rather than a contradiction.
func reachField() wizardField {
	options := []multiOption{{
		label:  "This machine only (localhost)",
		value:  "ws://localhost:" + strconv.Itoa(config.SFUPort),
		chosen: true,
	}}
	for _, address := range config.LocalAddresses() {
		options = append(options, multiOption{
			label: address.IP + "  (" + address.Label + ")",
			value: "ws://" + address.IP + ":" + strconv.Itoa(config.SFUPort),
		})
	}
	options = append(options, multiOption{label: "A domain or address I will type", value: domainChoice})
	return wizardField{
		key:     "reach",
		label:   "Where will people connect from?",
		helper:  "Space ticks, ↑/↓ moves. Tick every route that applies; clients use whichever answers fastest.",
		options: options,
	}
}

// domainChoice is a marker rather than an endpoint: ticking it reveals a field
// to type the real one into.
const domainChoice = "ask"

func selectField(key, label, helper string, choices []string, current int) wizardField {
	return wizardField{key: key, label: label, helper: helper, choices: choices, choice: current}
}

// recommend marks the choice to take when you have no reason to prefer
// another. Deliberately not on every question: path-style addressing is right
// for MinIO and wrong for AWS, so a badge there would be wrong half the time
// and would teach people to ignore it on the questions where it is right.
func recommend(field wizardField, choice string) wizardField {
	field.recommended = choice
	return field
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
	if len(f.options) > 0 {
		var chosen []string
		for _, option := range f.options {
			if option.chosen {
				chosen = append(chosen, option.value)
			}
		}
		return strings.Join(chosen, ",")
	}
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
	// Chosen here rather than left to the profile store, so a server created
	// now has one before anything tries to manage it.
	adminPort := config.FreeAdminPort(append(append([]int{}, taken...), config.FreePort(taken)))
	levels := config.SecurityLevels()
	security := make([]string, len(levels))
	for i, level := range levels {
		security[i] = string(level)
	}
	w := wizard{adminPort: adminPort, fields: []wizardField{
		inputField("name", "Server name", "Shown to people who connect.", "", "My Gryt Server"),
		inputField("host", "Bind address", "0.0.0.0 accepts connections from other machines. Use 127.0.0.1 to keep the server on this one.", "0.0.0.0", "0.0.0.0"),
		inputField("port", "Port", "TCP port exposed by Docker and used by clients. Offered because nothing else on this machine holds it.", port, port),
		recommend(selectField("security", "Security level", "Strict hides discovery; Community permits local identities.", security, 1), string(config.SecurityBalanced)),
		inputField("voice", "Voice seats", "0 means no limit, which is the server's own default. A cap is about your CPU and upload bandwidth, not about ports.", "0", "0"),
		inputField("proxy", "Trusted proxy hops", "Set to 1 for one reverse proxy or tunnel; otherwise leave 0.", "0", "0"),
		reachField(),
		onlyWhen(inputField("domain", "Its address", "Include the scheme. Behind a reverse proxy with TLS this is wss://, otherwise ws:// and the port.", "", "wss://voice.example.com"), "reach", domainChoice),
		recommend(selectField("storage", "Where do uploads go?", "Images, files and avatars people send to this server.", []string{"shared", "filesystem", "s3"}, 0), "shared"),

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
	chosen := map[string]bool{}
	for _, endpoint := range strings.Split(profile.SFUWebSocketURL, ",") {
		if endpoint != "" {
			chosen[endpoint] = true
		}
	}

	for i := range w.fields {
		field := &w.fields[i]
		if value, ok := values[field.key]; ok {
			field.input.SetValue(value)
		}
		if field.key == "reach" && profile.SFUWebSocketURL != "" {
			// An address this server uses that is not one of this machine's
			// current ones came from the typed field, so tick that and put it
			// back where it was entered.
			var extra []string
			for j := range field.options {
				known := chosen[field.options[j].value]
				field.options[j].chosen = known
				delete(chosen, field.options[j].value)
			}
			for endpoint := range chosen {
				extra = append(extra, endpoint)
			}
			if len(extra) > 0 {
				sort.Strings(extra)
				field.options[len(field.options)-1].chosen = true
				values["domain"] = strings.Join(extra, ",")
			}
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
	// Only text fields have an input to focus. A tick-list and a one-of-many
	// choice are built as bare structs, so their textinput is the zero value
	// and focusing it panics.
	field := w.fields[w.step]
	if len(field.choices) == 0 && len(field.options) == 0 {
		return w.fields[w.step].input.Focus()
	}
	return nil
}

func (w *wizard) update(msg tea.Msg) tea.Cmd {
	field := &w.fields[w.step]
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if len(field.options) > 0 {
			switch key.String() {
			case "up", "k":
				if field.cursor > 0 {
					field.cursor--
				}
			case "down", "j":
				if field.cursor < len(field.options)-1 {
					field.cursor++
				}
			case " ", "space", "x":
				field.options[field.cursor].chosen = !field.options[field.cursor].chosen
			}
			return nil
		}
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
		if other.key != field.whenKey {
			continue
		}
		if len(other.options) > 0 {
			// A tick-list holds several answers at once, so depending on one
			// of them is a membership test rather than an equality test.
			for _, option := range other.options {
				if option.chosen && option.value == field.whenValue {
					return true
				}
			}
			return false
		}
		return other.value() == field.whenValue
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
	case "reach":
		if value == "" {
			return fmt.Errorf("tick at least one way for people to connect")
		}
	case "domain":
		if value == "" {
			return fmt.Errorf("enter the address, or untick it on the previous step")
		}
		if !strings.HasPrefix(value, "ws://") && !strings.HasPrefix(value, "wss://") {
			return fmt.Errorf("start with ws:// or wss://")
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
	// The ticked routes, with the typed one substituted for its marker.
	var endpoints []string
	for _, endpoint := range strings.Split(values["reach"], ",") {
		if endpoint == "" {
			continue
		}
		if endpoint == domainChoice {
			if typed := values["domain"]; typed != "" {
				endpoints = append(endpoints, typed)
			}
			continue
		}
		endpoints = append(endpoints, endpoint)
	}
	profile.SFUWebSocketURL = strings.Join(endpoints, ",")
	profile.StorageBackend = values["storage"]
	if profile.AdminPort == 0 {
		profile.AdminPort = w.adminPort
	}
	return profile, profile.Validate()
}
