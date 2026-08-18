package app

import (
	"testing"

	"github.com/Gryt-chat/cli/internal/config"
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
	w := newWizard()
	if _, total := w.progress(); total != 8 {
		t.Fatalf("filesystem should ask 8 questions, got %d", total)
	}

	set(t, &w, "storage", "s3")
	if _, total := w.progress(); total != 14 {
		t.Fatalf("s3 should ask 14 questions, got %d", total)
	}
}

func TestStorageIsTheLastStepUntilS3IsChosen(t *testing.T) {
	w := newWizard()
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
	w := newWizard()
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
	w := newWizard()
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
