package runtime

import (
	"context"
	"io"

	"github.com/Gryt-chat/cli/internal/config"
)

type Fake struct {
	States        map[string]State
	Err           error
	Log           string
	SharedStarted bool
	Containers    map[string]bool
	Env           map[string]string
	// Project directories passed to Pull and PullShared, in the order they were pulled.
	Pulls []string
	// Fails the pulls alone, for testing what a half-finished update leaves behind.
	PullErr error
	// Profile ids passed to Start, so a test can tell a recreate from a pull that stopped.
	Starts []string
	// Merged into Env when Start runs, so a recreated container can report a new version.
	AfterStart map[string]string
	// Image ids by container name, and what EnsureShared leaves behind in them.
	Images      map[string]string
	AfterShared map[string]string
	// Image names by container name, tag included.
	ImageRefs map[string]string
}

func (f *Fake) Available(context.Context) error { return f.Err }
func (f *Fake) EnsureShared(_ context.Context, _ string) error {
	f.SharedStarted = true
	if f.Images == nil {
		f.Images = map[string]string{}
	}
	for name, image := range f.AfterShared {
		f.Images[name] = image
	}
	return f.Err
}
func (f *Fake) ContainerImageID(_ context.Context, name string) string {
	return f.Images[name]
}
func (f *Fake) ContainerImageRef(_ context.Context, name string) string {
	return f.ImageRefs[name]
}
func (f *Fake) Status(_ context.Context, p config.Profile) State {
	if state, ok := f.States[p.ID]; ok {
		return state
	}
	return StateStopped
}
func (f *Fake) Start(_ context.Context, p config.Profile, _ string) error {
	if f.States == nil {
		f.States = map[string]State{}
	}
	f.States[p.ID] = StateRunning
	f.Starts = append(f.Starts, p.ID)
	if f.Env == nil {
		f.Env = map[string]string{}
	}
	for key, value := range f.AfterStart {
		f.Env[key] = value
	}
	return f.Err
}
func (f *Fake) Pull(_ context.Context, _ config.Profile, dir string, _ io.Writer) error {
	f.Pulls = append(f.Pulls, dir)
	return f.PullErr
}
func (f *Fake) PullShared(_ context.Context, dir string, _ io.Writer) error {
	f.Pulls = append(f.Pulls, dir)
	return f.PullErr
}
func (f *Fake) Stop(_ context.Context, p config.Profile, _ string) error {
	if f.States == nil {
		f.States = map[string]State{}
	}
	f.States[p.ID] = StateStopped
	return f.Err
}
func (f *Fake) Restart(context.Context, config.Profile, string) error { return f.Err }
func (f *Fake) ContainerRunning(_ context.Context, name string) bool {
	return f.Containers[name]
}
func (f *Fake) ContainerEnv(_ context.Context, _ string, key string) string {
	return f.Env[key]
}
func (f *Fake) ContainerLogs(_ context.Context, _ string, _ int) (string, error) {
	return f.Log, f.Err
}
func (f *Fake) StopShared(context.Context, string) error {
	f.SharedStarted = false
	return f.Err
}
func (f *Fake) Logs(context.Context, config.Profile, string, int) (string, error) {
	return f.Log, f.Err
}
