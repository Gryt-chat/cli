package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Gryt-chat/cli/internal/config"
)

// failing returns a Probe that fails for the given argument, so a test can
// break one docker call without breaking the others.
func failing(arg string) Probe {
	return func(_ context.Context, _ string, args ...string) error {
		for _, a := range args {
			if a == arg {
				return errors.New("nope")
			}
		}
		return nil
	}
}

func ok(context.Context, string, ...string) error { return nil }

func find(checks []Check, name string) Check {
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	return Check{Name: "missing"}
}

// The bug this package exists for: `docker compose version` never contacts the
// daemon, so a stopped Docker Desktop used to pass the only check there was.
func TestAStoppedDaemonIsReportedEvenThoughComposeAnswers(t *testing.T) {
	checks := Docker(context.Background(), failing("info"))

	if compose := find(checks, "Compose plugin"); !compose.OK {
		t.Fatal("the compose plugin check should still pass")
	}
	daemon := find(checks, "Docker daemon")
	if daemon.OK {
		t.Fatal("a daemon that does not answer must not pass")
	}
	if daemon.Fix == "" {
		t.Fatal("a failing check has to say what to do about it")
	}
}

func TestAMissingComposePluginIsReported(t *testing.T) {
	if find(Docker(context.Background(), failing("compose")), "Compose plugin").OK {
		t.Fatal("a missing compose plugin must not pass")
	}
}

func TestProblemsReturnsOnlyFailures(t *testing.T) {
	problems := Problems([]Check{{Name: "a", OK: true}, {Name: "b"}, {Name: "c", OK: true}})
	if len(problems) != 1 || problems[0].Name != "b" {
		t.Fatalf("unexpected problems: %#v", problems)
	}
}

func TestEveryFailureCarriesAFix(t *testing.T) {
	checks := Environment(context.Background(), failing("info"), filepath.Join(t.TempDir(), "root"), nil)
	for _, problem := range Problems(checks) {
		if problem.Fix == "" {
			t.Fatalf("%q fails without saying what to do", problem.Name)
		}
	}
}

func TestConfigDirectoryThatCannotBeWrittenIsReported(t *testing.T) {
	root := filepath.Join(t.TempDir(), "readonly")
	if err := os.MkdirAll(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	check := find(Environment(context.Background(), ok, filepath.Join(root, "gryt"), nil), "Config directory")
	if check.OK {
		t.Skip("running as a user that can write through 0500, so there is nothing to detect")
	}
	if check.Fix == "" {
		t.Fatal("an unwritable config directory has to say what to do")
	}
}

// Two servers on 5000 is easy to reach, because the wizard defaults every new
// one to 5000 and nothing warned about it.
func TestTwoServersOnOnePortAreReported(t *testing.T) {
	first := config.NewProfile("First")
	second := config.NewProfile("Second")

	check := find(Environment(context.Background(), ok, t.TempDir(), []config.Profile{first, second}), "Ports")
	if check.OK {
		t.Fatal("two servers on 127.0.0.1:5000 should be reported")
	}
	if check.Fix == "" {
		t.Fatal("the clash has to say what to do about it")
	}
}

func TestDistinctPortsPass(t *testing.T) {
	first := config.NewProfile("First")
	second := config.NewProfile("Second")
	second.Port = 5001

	if !find(Environment(context.Background(), ok, t.TempDir(), []config.Profile{first, second}), "Ports").OK {
		t.Fatal("servers on different ports should pass")
	}
}
