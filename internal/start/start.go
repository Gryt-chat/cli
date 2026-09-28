// Package start brings one server's containers up — the "s" key in the manager — from the
// command line, so a script can do it without driving the TUI.
package start

import (
	"context"
	"fmt"
	"io"

	"github.com/Gryt-chat/cli/internal/config"
	"github.com/Gryt-chat/cli/internal/runtime"
)

type Options struct {
	ServerID string
}

// Run starts the shared voice server first, same as the manager does, then the named
// server. Returns the profile so a caller can print its addresses.
func Run(ctx context.Context, out io.Writer, store *config.Store, manager runtime.Manager, opts Options) (config.Profile, error) {
	profiles, err := store.List()
	if err != nil {
		return config.Profile{}, err
	}
	profile, ok := find(profiles, opts.ServerID)
	if !ok {
		return config.Profile{}, fmt.Errorf("server %q not found. Run gryt list to see the servers on this machine", opts.ServerID)
	}

	if err := manager.Available(ctx); err != nil {
		return config.Profile{}, err
	}

	// The SFU is shared by every server here, so it has to be up before a server
	// that expects to reach it starts.
	if _, err := store.WriteSharedCompose(); err != nil {
		return config.Profile{}, err
	}
	fmt.Fprintln(out, "Starting the shared voice server.")
	if err := manager.EnsureShared(ctx, store.SharedDir()); err != nil {
		return config.Profile{}, fmt.Errorf("starting the shared voice server: %w", err)
	}

	dir := store.ServerDir(profile.ID)
	// Rewritten so a server made by an older gryt picks up fixes to the file,
	// such as the data folder's owner.
	if _, err := store.WriteCompose(profile); err != nil {
		return config.Profile{}, err
	}
	fmt.Fprintf(out, "Starting %s.\n", profile.Name)
	if err := manager.Start(ctx, profile, dir); err != nil {
		return config.Profile{}, fmt.Errorf("starting %s: %w", profile.Name, err)
	}

	fmt.Fprintf(out, "\n%s is up on %s.\n", profile.Name, profile.Address())
	return profile, nil
}

func find(profiles []config.Profile, id string) (config.Profile, bool) {
	for _, profile := range profiles {
		if profile.ID == config.Slug(id) {
			return profile, true
		}
	}
	return config.Profile{}, false
}
