package runtime

import (
	"context"

	"github.com/Gryt-chat/cli/internal/config"
)

type Fake struct {
	States map[string]State
	Err    error
	Log    string
}

func (f *Fake) Available(context.Context) error { return f.Err }
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
	return f.Err
}
func (f *Fake) Stop(_ context.Context, p config.Profile, _ string) error {
	if f.States == nil {
		f.States = map[string]State{}
	}
	f.States[p.ID] = StateStopped
	return f.Err
}
func (f *Fake) Restart(context.Context, config.Profile, string) error { return f.Err }
func (f *Fake) Logs(context.Context, config.Profile, string, int) (string, error) {
	return f.Log, f.Err
}
