package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Gryt-chat/cli/internal/config"
	gruntime "github.com/Gryt-chat/cli/internal/runtime"
)

// set puts a value into the field with the given key, whichever kind it is.
func set(t *testing.T, w *wizard, key, value string) {
	t.Helper()
	for i := range w.fields {
		field := &w.fields[i]
		if field.key != key {
			continue
		}
		if len(field.choices) == 0 {
			field.input.SetValue(value)
			return
		}
		for choice, option := range field.choices {
			if option == value {
				field.choice = choice
				return
			}
		}
		t.Fatalf("field %q has no choice %q", key, value)
	}
	t.Fatalf("no field %q", key)
}

func TestStorageChoiceControlsHowManyStepsThereAre(t *testing.T) {
	w := newWizard(nil)
	if _, total := w.progress(); total != 8 {
		t.Fatalf("filesystem should ask 8 questions, got %d", total)
	}

	set(t, &w, "storage", "s3")
	if _, total := w.progress(); total != 14 {
		t.Fatalf("s3 should ask 14 questions, got %d", total)
	}
}

func TestStorageIsTheLastStepUntilS3IsChosen(t *testing.T) {
	w := newWizard(nil)
	set(t, &w, "name", "My Server")
	w.step = indexOf(t, w, "storage")

	if !w.complete() {
		t.Fatal("storage should be the final step for a filesystem server")
	}

	set(t, &w, "storage", "s3")
	if w.complete() {
		t.Fatal("storage must not be the final step once s3 is chosen")
	}

	w.step = indexOf(t, w, "s3path")
	if !w.complete() {
		t.Fatal("path-style should be the final step for an s3 server")
	}
}

func TestS3AnswersReachTheEnvironment(t *testing.T) {
	w := newWizard(nil)
	set(t, &w, "name", "My Server")
	set(t, &w, "storage", "s3")
	set(t, &w, "s3endpoint", "http://minio:9000")
	set(t, &w, "s3bucket", "uploads")
	set(t, &w, "s3key", "minioadmin")
	set(t, &w, "s3secret", "hunter2")

	profile, err := w.profile()
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"S3_ENDPOINT":          "http://minio:9000",
		"S3_BUCKET":            "uploads",
		"S3_REGION":            "auto",
		"S3_ACCESS_KEY_ID":     "minioadmin",
		"S3_SECRET_ACCESS_KEY": "hunter2",
		"S3_FORCE_PATH_STYLE":  "true",
	}
	for key, value := range want {
		if profile.ExtraEnv[key] != value {
			t.Fatalf("%s = %q, want %q", key, profile.ExtraEnv[key], value)
		}
	}

	// The whole point of the change: the generated .env has to carry the
	// credentials, not just the backend name.
	var sawSecret bool
	for _, setting := range profile.EnvSettings() {
		if setting.Key == "S3_SECRET_ACCESS_KEY" {
			sawSecret = true
			if !setting.Sensitive {
				t.Fatal("the secret access key must be marked sensitive")
			}
		}
	}
	if !sawSecret {
		t.Fatal("S3_SECRET_ACCESS_KEY never reached EnvSettings")
	}
}

func TestSwitchingAwayFromS3ClearsCredentialsButKeepsOtherKeys(t *testing.T) {
	existing := config.NewProfile("My Server")
	existing.StorageBackend = "s3"
	existing.ExtraEnv = map[string]string{
		"S3_ENDPOINT":          "http://minio:9000",
		"S3_BUCKET":            "uploads",
		"S3_ACCESS_KEY_ID":     "minioadmin",
		"S3_SECRET_ACCESS_KEY": "hunter2",
		"SOMETHING_ELSE":       "kept",
	}

	w := wizardFromProfile(existing)
	set(t, &w, "storage", "filesystem")

	profile, err := w.profile()
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range s3EnvKeys {
		if _, present := profile.ExtraEnv[key]; present {
			t.Fatalf("%s survived a switch to the filesystem backend", key)
		}
	}
	if profile.ExtraEnv["SOMETHING_ELSE"] != "kept" {
		t.Fatal("a key the wizard does not own was dropped")
	}
}

func TestEditingAnS3ServerKeepsItsCredentials(t *testing.T) {
	existing := config.NewProfile("My Server")
	existing.StorageBackend = "s3"
	existing.ExtraEnv = map[string]string{
		"S3_ENDPOINT":          "http://minio:9000",
		"S3_BUCKET":            "uploads",
		"S3_REGION":            "eu-central-1",
		"S3_ACCESS_KEY_ID":     "minioadmin",
		"S3_SECRET_ACCESS_KEY": "hunter2",
		"S3_FORCE_PATH_STYLE":  "false",
	}

	profile, err := wizardFromProfile(existing).profile()
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range existing.ExtraEnv {
		if profile.ExtraEnv[key] != value {
			t.Fatalf("%s = %q after a round trip, want %q", key, profile.ExtraEnv[key], value)
		}
	}
}

func TestS3StepsAreValidated(t *testing.T) {
	w := newWizard(nil)
	set(t, &w, "storage", "s3")

	w.step = indexOf(t, w, "s3endpoint")
	if err := w.validateStep(); err == nil {
		t.Fatal("an empty endpoint should not validate")
	}
	set(t, &w, "s3endpoint", "minio:9000")
	if err := w.validateStep(); err == nil {
		t.Fatal("an endpoint without a scheme should not validate")
	}
	set(t, &w, "s3endpoint", "http://minio:9000")
	if err := w.validateStep(); err != nil {
		t.Fatalf("a valid endpoint was rejected: %v", err)
	}
}

func indexOf(t *testing.T, w wizard, key string) int {
	t.Helper()
	for i, field := range w.fields {
		if field.key == key {
			return i
		}
	}
	t.Fatalf("no field %q", key)
	return -1
}

// Regression: the dashboard decided whether enter saves with
// `step == len(fields)-1`, which stopped meaning "the last visible step" as
// soon as fields became conditional. A filesystem server could be walked to
// step 8 of 8 and never saved, because the last field in the slice was the
// sixth S3 one.
func TestEnterSavesOnTheLastVisibleStepForBothBackends(t *testing.T) {
	for _, backend := range []string{"filesystem", "s3"} {
		w := newWizard(nil)
		set(t, &w, "name", "My Server")
		set(t, &w, "storage", backend)

		last := w.visible()[len(w.visible())-1]
		if w.fields[last].key == "storage" && backend == "s3" {
			t.Fatal("storage cannot be the last step for an s3 server")
		}

		w.step = last
		if !w.onLastStep() {
			t.Fatalf("%s: the last visible step is not recognised as last", backend)
		}

		// And the step before it must not be, or enter would save early.
		w.step = w.visible()[len(w.visible())-2]
		if w.onLastStep() {
			t.Fatalf("%s: the second-to-last step was treated as last", backend)
		}
	}
}

// The test that would actually have caught it: the bug lived in the dashboard's
// key handling, not in the wizard, so asserting on onLastStep alone proves
// nothing about whether enter is wired to it.
func TestPressingEnterOnTheLastStepSavesAFilesystemServer(t *testing.T) {
	model := New(config.NewStore(t.TempDir()), &gruntime.Fake{}, "v0.1.0")
	model.mode = modeWizard
	model.wizard = newWizard(nil)
	set(t, &model.wizard, "name", "My Server")
	steps := model.wizard.visible()
	model.wizard.step = steps[len(steps)-1]

	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on the last step produced no command, so nothing was saved")
	}

	// It stays on the step while the save runs, so the saving state has
	// somewhere to appear and a failure can be shown against the form.
	saving, ok := updated.(Model)
	if !ok || saving.mode != modeWizard || !saving.busy {
		t.Fatal("the wizard should stay open and busy while it saves")
	}

	done, _ := saving.Update(operationDone{message: "Saved My Server"})
	if next := done.(Model); next.mode != modeDashboard || next.busy {
		t.Fatal("a finished save should close the wizard")
	}
}

// A save that fails used to drop you on the dashboard with an error about a
// form you could no longer see.
func TestAFailedSaveKeepsYouOnTheStep(t *testing.T) {
	model := New(config.NewStore(t.TempDir()), &gruntime.Fake{}, "v0.4.0")
	model.mode = modeWizard
	model.wizard = newWizard(nil)
	model.busy = true

	updated, _ := model.Update(operationDone{err: errors.New("disk full")})
	next := updated.(Model)
	if next.mode != modeWizard {
		t.Fatal("a failed save left the wizard")
	}
	if !strings.Contains(next.wizard.err, "disk full") {
		t.Fatalf("the reason was not shown on the step: %q", next.wizard.err)
	}
}

// Defaults are placeholders now, so a field left alone is empty and still means
// its default.
func TestLeavingDefaultedFieldsAloneUsesTheDefaults(t *testing.T) {
	w := newWizard(nil)
	set(t, &w, "name", "My Server")

	profile, err := w.profile()
	if err != nil {
		t.Fatal(err)
	}
	if profile.Host != "0.0.0.0" {
		t.Fatalf("host = %q", profile.Host)
	}
	// The offered port is whatever is free here, not a fixed 5000.
	if profile.Port < config.DefaultPort {
		t.Fatalf("port = %d, below the starting point", profile.Port)
	}
	if profile.VoiceMaxUsers != 0 || profile.TrustedProxyHops != 0 {
		t.Fatalf("voice = %d, proxy = %d", profile.VoiceMaxUsers, profile.TrustedProxyHops)
	}
}

// The bug: the field arrived holding "5000" with the cursor at the end, so
// typing appended and you got 50005001. Typing now replaces because there is
// nothing there to append to.
func TestTypingIntoADefaultedFieldReplacesRatherThanAppends(t *testing.T) {
	w := newWizard(nil)
	set(t, &w, "name", "My Server")
	set(t, &w, "port", "5001")

	profile, err := w.profile()
	if err != nil {
		t.Fatal(err)
	}
	if profile.Port != 5001 {
		t.Fatalf("port = %d, want 5001", profile.Port)
	}
}

func TestAnEmptyDefaultedFieldValidates(t *testing.T) {
	w := newWizard(nil)
	for _, key := range []string{"host", "port", "voice", "proxy"} {
		w.step = indexOf(t, w, key)
		if err := w.validateStep(); err != nil {
			t.Fatalf("%s: leaving the default alone should validate, got %v", key, err)
		}
	}
}

// A field with no default is still required.
func TestFieldsWithoutADefaultAreStillRequired(t *testing.T) {
	w := newWizard(nil)
	w.step = indexOf(t, w, "name")
	if err := w.validateStep(); err == nil {
		t.Fatal("an empty server name should not validate")
	}

	set(t, &w, "storage", "s3")
	w.step = indexOf(t, w, "s3endpoint")
	if err := w.validateStep(); err == nil {
		t.Fatal("an empty S3 endpoint should not validate")
	}
}

// Editing is the other direction: there the current value is what you want to
// see, so it is filled in rather than hinted at.
func TestEditingShowsTheCurrentValues(t *testing.T) {
	existing := config.NewProfile("My Server")
	existing.Port = 5005
	existing.Host = "127.0.0.1"

	w := wizardFromProfile(existing)
	if got := w.fields[indexOf(t, w, "port")].input.Value(); got != "5005" {
		t.Fatalf("port field shows %q when editing", got)
	}

	profile, err := w.profile()
	if err != nil {
		t.Fatal(err)
	}
	if profile.Port != 5005 || profile.Host != "127.0.0.1" {
		t.Fatalf("edit round trip changed the values: %d %s", profile.Port, profile.Host)
	}
}

// Starting a server has to bring the shared SFU up first, or the server comes
// up pointing at a container that does not exist.
func TestStartingAServerBringsUpTheSharedStack(t *testing.T) {
	store := config.NewStore(t.TempDir())
	profile := config.NewProfile("My Server")
	if err := store.Save(profile); err != nil {
		t.Fatal(err)
	}

	fake := &gruntime.Fake{}
	model := New(store, fake, "v0.1.5")
	model.profiles = []config.Profile{profile}

	cmd := model.runOperation("start", profile)
	if cmd == nil {
		t.Fatal("start produced no command")
	}
	cmd()

	if !fake.SharedStarted {
		t.Fatal("starting a server did not bring up the shared SFU")
	}
	if fake.States[profile.ID] != gruntime.StateRunning {
		t.Fatal("the server itself was not started")
	}
}

// Focusing a field that has no text input panicked: a tick-list is built as a
// bare struct, so its textinput is the zero value. The wizard died on the way
// into the step rather than while drawing it, which is why rendering it in
// isolation looked fine.
func TestFocusingEveryStepDoesNotPanic(t *testing.T) {
	w := newWizard(nil)
	set(t, &w, "storage", "s3")
	for _, step := range w.visible() {
		w.step = step
		w.focus()
	}
}

func tickOption(t *testing.T, w *wizard, label string) {
	t.Helper()
	field := &w.fields[indexOf(t, *w, "reach")]
	for i := range field.options {
		if strings.Contains(field.options[i].label, label) {
			field.options[i].chosen = true
			return
		}
	}
	t.Fatalf("no reach option matching %q", label)
}

func TestReachOffersLocalhostAndTheMachinesAddresses(t *testing.T) {
	w := newWizard(nil)
	options := w.fields[indexOf(t, w, "reach")].options

	if len(options) < 2 {
		t.Fatal("reach should offer at least localhost and a way to type an address")
	}
	if !options[0].chosen {
		t.Fatal("localhost should start ticked, so the wizard has a working answer by default")
	}
	if options[len(options)-1].value != domainChoice {
		t.Fatal("the last option should be the one that reveals a field to type into")
	}
	for _, address := range config.LocalAddresses() {
		found := false
		for _, option := range options {
			if strings.Contains(option.value, address.IP) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s is an address of this machine and was not offered", address.IP)
		}
	}
}

// SFU_PUBLIC_HOST takes a list; the client pings each and uses the fastest. So
// ticking more than one is the point, not a contradiction.
func TestTickedRoutesBecomeACommaSeparatedPublicHost(t *testing.T) {
	w := newWizard(nil)
	set(t, &w, "name", "My Server")

	addresses := config.LocalAddresses()
	if len(addresses) == 0 {
		t.Skip("no non-virtual addresses on this machine to tick")
	}
	tickOption(t, &w, addresses[0].IP)

	profile, err := w.profile()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(profile.SFUWebSocketURL, ",")
	if len(parts) != 2 {
		t.Fatalf("expected localhost and one address, got %q", profile.SFUWebSocketURL)
	}
	if !strings.Contains(profile.SFUWebSocketURL, addresses[0].IP) {
		t.Fatalf("the ticked address is missing from %q", profile.SFUWebSocketURL)
	}
}

// The domain option is a marker, not an endpoint. It must never reach the file.
func TestTheTypedAddressReplacesItsMarker(t *testing.T) {
	w := newWizard(nil)
	set(t, &w, "name", "My Server")
	tickOption(t, &w, "domain or address")
	set(t, &w, "domain", "wss://voice.example.com")

	profile, err := w.profile()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(profile.SFUWebSocketURL, domainChoice) {
		t.Fatalf("the marker leaked into the configuration: %q", profile.SFUWebSocketURL)
	}
	if !strings.Contains(profile.SFUWebSocketURL, "wss://voice.example.com") {
		t.Fatalf("the typed address is missing from %q", profile.SFUWebSocketURL)
	}
}

func TestReachAndDomainAreValidated(t *testing.T) {
	w := newWizard(nil)
	field := &w.fields[indexOf(t, w, "reach")]
	for i := range field.options {
		field.options[i].chosen = false
	}
	w.step = indexOf(t, w, "reach")
	if err := w.validateStep(); err == nil {
		t.Fatal("ticking nothing should not validate")
	}

	tickOption(t, &w, "domain or address")
	w.step = indexOf(t, w, "domain")
	set(t, &w, "domain", "voice.example.com")
	if err := w.validateStep(); err == nil {
		t.Fatal("an address without a scheme should not validate")
	}
	set(t, &w, "domain", "wss://voice.example.com")
	if err := w.validateStep(); err != nil {
		t.Fatalf("a valid address was rejected: %v", err)
	}
}

// Editing has to put the ticks back, including one that came from the typed
// field rather than from this machine's own addresses.
func TestEditingRestoresTheTicksAndTheTypedAddress(t *testing.T) {
	existing := config.NewProfile("My Server")
	existing.SFUWebSocketURL = "ws://localhost:5005,wss://voice.example.com"

	w := wizardFromProfile(existing)
	field := w.fields[indexOf(t, w, "reach")]

	if !field.options[0].chosen {
		t.Fatal("localhost was not ticked back")
	}
	if !field.options[len(field.options)-1].chosen {
		t.Fatal("the typed-address option was not ticked back")
	}

	profile, err := w.profile()
	if err != nil {
		t.Fatal(err)
	}
	if profile.SFUWebSocketURL != existing.SFUWebSocketURL {
		t.Fatalf("a round trip changed it: %q became %q", existing.SFUWebSocketURL, profile.SFUWebSocketURL)
	}
}

func TestRecommendationsPointAtARealChoice(t *testing.T) {
	w := newWizard(nil)
	for _, field := range w.fields {
		if field.recommended == "" {
			continue
		}
		found := false
		for _, choice := range field.choices {
			if choice == field.recommended {
				found = true
			}
		}
		if !found {
			t.Fatalf("%q recommends %q, which is not one of its choices %v",
				field.key, field.recommended, field.choices)
		}
	}
}

// The recommendation should also be what the wizard already has selected, or
// it is advice the wizard itself does not take.
func TestTheRecommendedChoiceIsTheOneAlreadySelected(t *testing.T) {
	w := newWizard(nil)
	for _, field := range w.fields {
		if field.recommended == "" {
			continue
		}
		if field.choices[field.choice] != field.recommended {
			t.Fatalf("%q starts on %q but recommends %q",
				field.key, field.choices[field.choice], field.recommended)
		}
	}
}

// Not every question has a right answer. Path-style addressing is right for
// MinIO and wrong for AWS, so badging it would be wrong half the time and
// would teach people to ignore the badge where it is right.
func TestQuestionsWithoutARightAnswerCarryNoRecommendation(t *testing.T) {
	w := newWizard(nil)
	if got := w.fields[indexOf(t, w, "s3path")].recommended; got != "" {
		t.Fatalf("path-style addressing recommends %q; it depends on the provider", got)
	}
}

// 0.0.0.0 is an instruction to the kernel, not something to hand anybody. The
// panel showed it as the server's address, which answered the wrong question.
func TestJoinAddressesExpandTheWildcardBind(t *testing.T) {
	m := New(config.NewStore(t.TempDir()), &gruntime.Fake{}, "v0.3.0")
	profile := config.NewProfile("Test")
	profile.Host, profile.Port = "0.0.0.0", 5001

	lines := m.joinAddresses(profile)
	if len(lines) == 0 {
		t.Fatal("no address to give anybody")
	}
	for _, line := range lines {
		if strings.Contains(line, "0.0.0.0") {
			t.Fatalf("the wildcard bind was offered as a join address: %q", line)
		}
		if !strings.Contains(line, ":5001") {
			t.Fatalf("%q does not name the port", line)
		}
	}
}

func TestASpecificBindIsShownAsItself(t *testing.T) {
	m := New(config.NewStore(t.TempDir()), &gruntime.Fake{}, "v0.3.0")
	profile := config.NewProfile("Test")
	profile.Host, profile.Port = "127.0.0.1", 5001

	lines := m.joinAddresses(profile)
	if len(lines) != 1 || !strings.Contains(lines[0], "127.0.0.1:5001") {
		t.Fatalf("expected the bind address itself, got %v", lines)
	}
}

// A server can be running perfectly while voice is dead, because the SFU lives
// in a separate project. The panel said nothing about that.
func TestVoiceSaysWhenTheSharedServerIsDown(t *testing.T) {
	m := New(config.NewStore(t.TempDir()), &gruntime.Fake{}, "v0.3.0")
	profile := config.NewProfile("Test")
	profile.SFUWebSocketURL = "ws://localhost:5005"

	m.sharedUp = false
	if !strings.Contains(m.voiceLine(profile), "not running") {
		t.Fatalf("a down SFU is not reported: %q", m.voiceLine(profile))
	}

	m.sharedUp = true
	if !strings.Contains(m.voiceLine(profile), "ready") {
		t.Fatalf("a working setup is not reported as ready: %q", m.voiceLine(profile))
	}

	profile.SFUWebSocketURL = ""
	if !strings.Contains(m.voiceLine(profile), "no route") {
		t.Fatalf("a server with no route is not reported: %q", m.voiceLine(profile))
	}
}

// The dashboard used to show whatever the status had been when you last
// pressed g, so a server that fell over looked fine until you thought to ask.
func TestATickRefreshesTheDashboard(t *testing.T) {
	store := config.NewStore(t.TempDir())
	profile := config.NewProfile("Test")
	if err := store.Save(profile); err != nil {
		t.Fatal(err)
	}

	m := New(store, &gruntime.Fake{}, "v0.3.0")
	m.profiles = []config.Profile{profile}

	updated, cmd := m.Update(tick{})
	if cmd == nil {
		t.Fatal("a tick produced no work, so the dashboard would never refresh again")
	}
	if !updated.(Model).refreshing {
		t.Fatal("the tick did not start a refresh")
	}
}

// A slow health check must not have ticks stacking up behind it.
func TestATickDuringARefreshOnlyReschedules(t *testing.T) {
	store := config.NewStore(t.TempDir())
	profile := config.NewProfile("Test")
	m := New(store, &gruntime.Fake{}, "v0.3.0")
	m.profiles = []config.Profile{profile}
	m.refreshing = true

	updated, cmd := m.Update(tick{})
	if cmd == nil {
		t.Fatal("the loop must keep ticking even while busy")
	}
	if !updated.(Model).refreshing {
		t.Fatal("an in-flight refresh was cancelled by a tick")
	}
}

// A container that goes away mid-follow should not replace the logs on screen
// with an error.
func TestFollowingLogsKeepsWhatIsOnScreenWhenItFails(t *testing.T) {
	m := New(config.NewStore(t.TempDir()), &gruntime.Fake{}, "v0.3.0")
	m.logs = "existing output"

	updated, _ := m.Update(logsFollowed{content: ""})
	if updated.(Model).logs != "existing output" {
		t.Fatal("a failed follow wiped the logs already on screen")
	}

	updated, _ = updated.(Model).Update(logsFollowed{content: "newer output"})
	if updated.(Model).logs != "newer output" {
		t.Fatal("a successful follow did not update the logs")
	}
}

// The panel headed "Give people this address" led with 127.0.0.1, which is the
// one address nobody else can use.
func TestReachableAddressesComeBeforeLoopback(t *testing.T) {
	m := New(config.NewStore(t.TempDir()), &gruntime.Fake{}, "v0.3.1")
	profile := config.NewProfile("Test")
	profile.Host, profile.Port = "0.0.0.0", 5001

	lines := m.joinAddresses(profile)
	if len(lines) < 2 {
		t.Skip("no non-loopback address on this machine to rank above loopback")
	}
	if strings.Contains(lines[0], "127.0.0.1") {
		t.Fatalf("loopback is first, so the primary address is one nobody can use: %q", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], "127.0.0.1") {
		t.Fatalf("loopback should be last, got %q", lines[len(lines)-1])
	}
}

// The detail view swallowed every key but esc, while its own footer listed
// s, x, r and l — naming keys that did nothing on the one screen dedicated to
// that server.
func TestTheDetailViewCanActOnItsServer(t *testing.T) {
	store := config.NewStore(t.TempDir())
	profile := config.NewProfile("Test")
	if err := store.Save(profile); err != nil {
		t.Fatal(err)
	}

	fake := &gruntime.Fake{}
	model := New(store, fake, "v0.4.1")
	model.profiles = []config.Profile{profile}
	model.mode = modeDetail

	updated, cmd := model.Update(tea.KeyPressMsg{Code: 's'})
	if cmd == nil {
		t.Fatal("s did nothing in the detail view")
	}
	cmd()
	if fake.States[profile.ID] != gruntime.StateRunning {
		t.Fatal("s in the detail view did not start the server")
	}
	if next := updated.(Model); next.mode != modeDetail {
		t.Fatal("acting on the server should leave you where you were")
	}
}

func TestEscapeStillLeavesTheDetailView(t *testing.T) {
	model := New(config.NewStore(t.TempDir()), &gruntime.Fake{}, "v0.4.1")
	model.mode = modeDetail

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next := updated.(Model); next.mode != modeDashboard {
		t.Fatal("esc did not return to the table")
	}
}
