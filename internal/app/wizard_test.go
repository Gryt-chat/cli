package app

import (
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
	if next, ok := updated.(Model); !ok || next.mode != modeDashboard {
		t.Fatal("enter on the last step should leave the wizard")
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
