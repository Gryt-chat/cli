package app

import (
	"strings"
	"testing"

	"github.com/Gryt-chat/cli/internal/config"
	gruntime "github.com/Gryt-chat/cli/internal/runtime"
)

func TestEmptyDashboardExplainsNextAction(t *testing.T) {
	model := New(config.NewStore(t.TempDir()), &gruntime.Fake{}, "v0.1.0")
	view := model.viewDashboard()
	if !containsAll(view, "No servers configured", "Press n") {
		t.Fatalf("empty dashboard lacks next action:\n%s", view)
	}
}

func TestWizardProducesValidatedProfile(t *testing.T) {
	w := newWizard()
	w.fields[0].input.SetValue("My Server")
	profile, err := w.profile()
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "my-server" || profile.Port != 5000 || profile.Security != config.SecurityBalanced {
		t.Fatalf("unexpected profile: %#v", profile)
	}
}

func containsAll(value string, wants ...string) bool {
	for _, want := range wants {
		if !strings.Contains(value, want) {
			return false
		}
	}
	return true
}
