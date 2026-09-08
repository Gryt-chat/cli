package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
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
	// ContainerRunning reports whether one named container is up. The shared project's
	// pieces are addressed by container name because they are not any server's.
	ContainerRunning(context.Context, string) bool
	// ContainerLogs reads one container's output, for the same reason.
	ContainerLogs(context.Context, string, int) (string, error)
	// ContainerEnv reads one variable out of a running container.
	ContainerEnv(context.Context, string, string) string
	// StopShared takes the shared project down.
	StopShared(context.Context, string) error
}

type Docker struct{}

// Available reports the first thing standing between the operator and a running container.
// The checks live in internal/doctor, so this and `gryt doctor` cannot disagree.
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
	return composeCommandEnv(ctx, dir, nil, args...)
}

// composeCommandEnv runs compose with extra environment of its own. The management token
// reaches the container this way rather than through .env, which people paste into reports.
func composeCommandEnv(ctx context.Context, dir string, env []string, args ...string) error {
	base := []string{"compose", "--project-directory", dir, "--file", dir + "/compose.yaml"}
	cmd := exec.CommandContext(ctx, "docker", append(base, args...)...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose: %s", composeReason(stderr.String(), err))
	}
	return nil
}

// composeReason picks the line worth showing out of compose's output. The reason is at the
// end; returning the whole buffer showed "Container X Creating" for a failed start.
func composeReason(output string, fallback error) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") ||
			strings.Contains(lower, "cannot") || strings.Contains(lower, "no such") {
			return line
		}
	}

	// Nothing announced itself as an error, so the last thing it said is the
	// best available account of where it stopped.
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return fallback.Error()
}

func (Docker) EnsureShared(ctx context.Context, dir string) error {
	return composeCommand(ctx, dir, "up", "--detach", "--remove-orphans")
}

func (Docker) Start(ctx context.Context, profile config.Profile, dir string) error {
	return composeCommandEnv(ctx, dir, adminEnv(profile), "up", "--detach", "--remove-orphans")
}

// adminEnv carries the management token into the compose invocation. Empty when the profile
// has none, in which case the server starts no management listener at all.
func adminEnv(profile config.Profile) []string {
	if profile.AdminToken == "" {
		return nil
	}
	return []string{"GRYT_ADMIN_TOKEN=" + profile.AdminToken}
}

func (Docker) Stop(ctx context.Context, _ config.Profile, dir string) error {
	return composeCommand(ctx, dir, "down")
}

func (Docker) Restart(ctx context.Context, profile config.Profile, dir string) error {
	return composeCommandEnv(ctx, dir, adminEnv(profile), "restart")
}

func (Docker) ContainerRunning(ctx context.Context, name string) bool {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", name)
	out, err := cmd.Output()
	if err != nil {
		// No such container, which for our purposes is the same as not running.
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// ContainerEnv reads a variable out of a running container. The image bakes SERVER_VERSION
// in at build time, so this works on images built before the server could report it.
func (Docker) ContainerEnv(ctx context.Context, name, key string) string {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "--format",
		"{{range .Config.Env}}{{println .}}{{end}}", name)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), key+"="); ok {
			return value
		}
	}
	return ""
}

func (Docker) ContainerLogs(ctx context.Context, name string, lines int) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", strconv.Itoa(lines), name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker logs %s: %s", name, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (Docker) StopShared(ctx context.Context, dir string) error {
	return composeCommand(ctx, dir, "down")
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
