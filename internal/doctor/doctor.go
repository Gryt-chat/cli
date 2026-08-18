// Package doctor answers "why did that not work" before the operator has to
// ask it.
//
// Everything here is about the machine rather than about Gryt: whether Docker
// is installed, whether its daemon is up, whether the config directory can be
// written. The CLI used to find all of this out by running docker compose and
// showing whatever came back, which is accurate and unreadable.
package doctor

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/Gryt-chat/cli/internal/config"
)

// Check is one thing that can be wrong, and the one thing to do about it.
type Check struct {
	Name   string
	OK     bool
	Detail string
	// Fix is a command to run or an action to take. Empty when the check passed.
	Fix string
}

// Probe runs a command and reports only whether it succeeded. Injected so the
// tests do not need Docker installed to cover the branches.
type Probe func(ctx context.Context, name string, args ...string) error

// Exec is the real Probe.
func Exec(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// Environment reports on the machine. Profiles are used only for checks that
// compare servers against each other, so passing none is fine.
func Environment(ctx context.Context, probe Probe, root string, profiles []config.Profile) []Check {
	checks := append(Docker(ctx, probe), configWritable(root))
	if duplicate := duplicatePorts(profiles); duplicate != nil {
		checks = append(checks, *duplicate)
	}
	if squatted := squattedPorts(ctx, profiles); squatted != nil {
		checks = append(checks, *squatted)
	}
	return checks
}

// Problems returns only the failures, which is what a caller usually wants.
func Problems(checks []Check) []Check {
	var problems []Check
	for _, check := range checks {
		if !check.OK {
			problems = append(problems, check)
		}
	}
	return problems
}

// Docker is the subset that decides whether a deployment can be started at
// all. Kept separate so the pre-flight check before an action and the doctor
// command cannot drift apart.
func Docker(ctx context.Context, probe Probe) []Check {
	return []Check{dockerInstalled(), composePlugin(ctx, probe), daemonRunning(ctx, probe)}
}

func dockerInstalled() Check {
	check := Check{Name: "Docker installed"}
	path, err := exec.LookPath("docker")
	if err != nil {
		check.Detail = "no docker on PATH"
		check.Fix = installHint()
		return check
	}
	check.OK, check.Detail = true, path
	return check
}

func composePlugin(ctx context.Context, probe Probe) Check {
	check := Check{Name: "Compose plugin"}
	if err := probe(ctx, "docker", "compose", "version"); err != nil {
		check.Detail = "docker compose is not available"
		check.Fix = "Install the Compose plugin: https://docs.docker.com/compose/install/"
		return check
	}
	check.OK, check.Detail = true, "available"
	return check
}

// The check the old one should have been. `docker compose version` asks the
// client about itself and never contacts the daemon, so it passes with Docker
// Desktop installed and shut down, which on macOS is the single most likely
// thing to be wrong. `docker info` is the cheapest call that needs the daemon.
func daemonRunning(ctx context.Context, probe Probe) Check {
	check := Check{Name: "Docker daemon"}
	if err := probe(ctx, "docker", "info"); err != nil {
		check.Detail = "installed, but not running"
		check.Fix = startHint()
		return check
	}
	check.OK, check.Detail = true, "running"
	return check
}

func configWritable(root string) Check {
	check := Check{Name: "Config directory", Detail: root}
	if err := os.MkdirAll(root, 0o700); err != nil {
		check.Detail = root + ": " + err.Error()
		check.Fix = "Set GRYT_CONFIG_DIR to a directory you can write to"
		return check
	}
	probe := filepath.Join(root, ".write-test")
	if err := os.WriteFile(probe, []byte(""), 0o600); err != nil {
		check.Detail = root + ": not writable"
		check.Fix = "Set GRYT_CONFIG_DIR to a directory you can write to"
		return check
	}
	_ = os.Remove(probe)
	check.OK = true
	return check
}

// Two servers on one port is a configuration mistake rather than a machine
// one, and it is invisible until the second container fails to bind. The
// wizard defaults every new server to 5000, so it is easy to arrive at.
func duplicatePorts(profiles []config.Profile) *Check {
	seen := map[string]string{}
	for _, profile := range profiles {
		key := profile.Address()
		if first, clash := seen[key]; clash {
			return &Check{
				Name:   "Ports",
				Detail: first + " and " + profile.Name + " both use " + key,
				Fix:    "Edit one of them with e and give it a different port",
			}
		}
		seen[key] = profile.Name
	}
	if len(profiles) == 0 {
		return nil
	}
	return &Check{Name: "Ports", OK: true, Detail: strconv.Itoa(len(profiles)) + " server(s), no clashes"}
}

// squattedPorts finds a server whose port is held by something that is not it.
//
// The discriminator is the answer, not whether the port can be bound.
//
// Binding was the first attempt and it was wrong: macOS allowed a bind of
// 127.0.0.1:5000 while ControlCenter held *:5000 for AirPlay, so the check
// concluded the port was free and never asked. Asking is the whole point. A
// Gryt server replies to /health with 2xx; AirTunes replies 403; a stopped
// server refuses the connection, which is not a problem and is skipped.
func squattedPorts(ctx context.Context, profiles []config.Profile) *Check {
	client := http.Client{Timeout: time.Second}
	for _, profile := range profiles {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+profile.Address()+"/health", nil)
		if err != nil {
			continue
		}
		res, err := client.Do(req)
		if err != nil {
			// Nothing answered, so nothing is squatting. A stopped server
			// looks exactly like this and is not a problem.
			continue
		}
		_ = res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return portTaken(profile, "something there answered "+res.Status)
		}
	}
	if len(profiles) == 0 {
		return nil
	}
	return &Check{Name: "Port owners", OK: true, Detail: "no server's port is held by anything else"}
}

func portTaken(profile config.Profile, why string) *Check {
	return &Check{
		Name:   "Port owners",
		Detail: profile.Name + " uses " + profile.Address() + " and " + why,
		Fix:    "Edit it with e and pick another port. On macOS, 5000 belongs to AirPlay Receiver.",
	}
}

func installHint() string {
	if runtime.GOOS == "linux" {
		return "Install Docker Engine: https://docs.docker.com/engine/install/"
	}
	return "Install Docker Desktop: https://docs.docker.com/desktop/"
}

func startHint() string {
	if runtime.GOOS == "linux" {
		return "Start it: sudo systemctl start docker"
	}
	return "Open Docker Desktop and wait for it to finish starting"
}
