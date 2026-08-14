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
	Start(context.Context, config.Profile, string) error
	Stop(context.Context, config.Profile, string) error
	Restart(context.Context, config.Profile, string) error
	Logs(context.Context, config.Profile, string, int) (string, error)
}

type Docker struct{}

func (Docker) Available(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "version")
	if err := cmd.Run(); err != nil {
		return errors.New("Docker Compose is not available; install Docker Desktop or the Compose plugin")
	}
	return nil
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
