package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Gryt-chat/cli/internal/config"
	"github.com/Gryt-chat/cli/internal/doctor"
)

type State string

const (
	StateRunning State = "running"
	StateStopped State = "stopped"
	StateUnknown State = "unknown"
)

type Manager interface {
	Available(context.Context) error
	Status(context.Context, config.Profile) State
	// EnsureShared brings up the project holding the SFU. Starting a server
	// without it leaves the server unable to reach any media plane at all.
	EnsureShared(context.Context, string) error
	Start(context.Context, config.Profile, string) error
	Stop(context.Context, config.Profile, string) error
	Restart(context.Context, config.Profile, string) error
	Logs(context.Context, config.Profile, string, int) (string, error)
}

type Docker struct{}

// Available reports the first thing standing between the operator and a
// running container.
//
// This used to run `docker compose version` alone, which asks the client about
// itself and never contacts the daemon: it passes with Docker Desktop
// installed and shut down, which on macOS is the most likely thing to be
// wrong. The checks live in internal/doctor so that this and `gryt doctor`
// cannot disagree.
func (Docker) Available(ctx context.Context) error {
	problems := doctor.Problems(doctor.Docker(ctx, doctor.Exec))
	if len(problems) == 0 {
		return nil
	}
	first := problems[0]
	if first.Fix == "" {
		return errors.New(first.Detail)
	}
	return fmt.Errorf("%s. %s", first.Detail, first.Fix)
}

func (Docker) Status(ctx context.Context, profile config.Profile) State {
	client := http.Client{Timeout: 900 * time.Millisecond}
	url := "http://" + profile.Address() + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return StateUnknown
	}
	res, err := client.Do(req)
	if err != nil {
		return StateStopped
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return StateRunning
	}
	return StateUnknown
}

func composeCommand(ctx context.Context, dir string, args ...string) error {
	base := []string{"compose", "--project-directory", dir, "--file", dir + "/compose.yaml"}
	cmd := exec.CommandContext(ctx, "docker", append(base, args...)...)
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("docker compose: %s", message)
	}
	return nil
}

func (Docker) EnsureShared(ctx context.Context, dir string) error {
	return composeCommand(ctx, dir, "up", "--detach", "--remove-orphans")
}

func (Docker) Start(ctx context.Context, _ config.Profile, dir string) error {
	return composeCommand(ctx, dir, "up", "--detach", "--remove-orphans")
}

func (Docker) Stop(ctx context.Context, _ config.Profile, dir string) error {
	return composeCommand(ctx, dir, "down")
}

func (Docker) Restart(ctx context.Context, _ config.Profile, dir string) error {
	return composeCommand(ctx, dir, "restart")
}

func (Docker) Logs(ctx context.Context, _ config.Profile, dir string, lines int) (string, error) {
	base := []string{"compose", "--project-directory", dir, "--file", dir + "/compose.yaml", "logs", "--no-color", "--tail", strconv.Itoa(lines)}
	cmd := exec.CommandContext(ctx, "docker", base...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker compose logs: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
