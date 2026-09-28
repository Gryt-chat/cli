// Package remove deletes a server from this machine: its containers, its folder, and the
// shared services once no server is left to use them.
package remove

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Gryt-chat/cli/internal/config"
	"github.com/Gryt-chat/cli/internal/runtime"
)

// Ask shows a prompt and returns what was typed. A parameter so tests answer without a
// terminal, and so a closed stdin reads as no answer rather than as a yes.
type Ask func(prompt string) (string, error)

type Options struct {
	ServerID string
	// Skips the question, for scripts. Nothing else changes.
	Yes bool
}

// Result says what went, so the caller can point at what it could not touch itself.
type Result struct {
	Profile config.Profile
	// True when this was the last server, and the shared services went with it.
	Last bool
}

// ErrDeclined is returned when the answer to the question was not the server's id.
var ErrDeclined = errors.New("nothing was removed")

func Run(ctx context.Context, out io.Writer, store *config.Store, manager runtime.Manager, ask Ask, opts Options) (Result, error) {
	profiles, err := store.List()
	if err != nil {
		return Result{}, err
	}
	profile, ok := find(profiles, opts.ServerID)
	if !ok {
		return Result{}, fmt.Errorf("server %q not found. Run gryt list to see the servers on this machine", opts.ServerID)
	}
	last := len(profiles) == 1
	dir := store.ServerDir(profile.ID)
	shared := store.SharedDir()

	fmt.Fprintf(out, "This deletes %s for good:\n", profile.Name)
	fmt.Fprintf(out, "  its containers, gryt-%s and gryt-%s-image-worker\n", profile.ID, profile.ID)
	fmt.Fprintf(out, "  %s, which holds its database, its uploads and its settings\n", dir)
	if profile.StorageBackend == config.SharedStorage && !last {
		fmt.Fprintln(out, "  Its uploads in the shared object store stay, because other servers here use that store too.")
	}
	if last {
		fmt.Fprintln(out, "It's the last server here, so the shared services go too:")
		fmt.Fprintf(out, "  the voice server (%s), the %s network, and %s\n", config.SFUContainer, config.SharedNetwork, shared)
		if store.UsesSharedStore() {
			fmt.Fprintf(out, "  the object store (%s) and every upload in it\n", config.MinIOContainer)
		}
	}
	fmt.Fprintln(out)

	if !opts.Yes {
		answer, err := ask(fmt.Sprintf("Type %s to delete it: ", profile.ID))
		if err != nil || strings.TrimSpace(answer) != profile.ID {
			return Result{}, ErrDeclined
		}
	}

	if err := manager.Available(ctx); err != nil {
		return Result{}, fmt.Errorf("%w. Nothing was removed", err)
	}

	// Read before the container goes: it is the image that is certainly on this machine.
	image := manager.ContainerImageRef(ctx, "gryt-"+profile.ID)
	if image == "" {
		image = "ghcr.io/gryt-chat/server:" + store.Preferences().ImageTag()
	}

	if exists(filepath.Join(dir, "compose.yaml")) {
		fmt.Fprintf(out, "Removing the containers for %s.\n", profile.Name)
		if err := manager.Remove(ctx, dir); err != nil {
			return Result{}, fmt.Errorf("%w. Its folder was left alone, so running this again is safe", err)
		}
	}

	fmt.Fprintf(out, "Deleting %s.\n", dir)
	if err := deleteFolder(ctx, manager, dir, image); err != nil {
		return Result{}, err
	}

	if last {
		if exists(filepath.Join(shared, "compose.yaml")) {
			fmt.Fprintln(out, "Removing the shared services.")
			if err := manager.RemoveShared(ctx, shared); err != nil {
				return Result{Profile: profile}, fmt.Errorf("%s is gone, but the shared services are not: %w", profile.Name, err)
			}
		}
		if err := os.RemoveAll(shared); err != nil {
			return Result{Profile: profile}, err
		}
		// Only when empty: preferences.json, or anything else put there, keeps them.
		_ = os.Remove(filepath.Dir(dir))
		_ = os.Remove(store.Root())
	}

	fmt.Fprintf(out, "\nRemoved %s.\n", profile.Name)
	return Result{Profile: profile, Last: last}, nil
}

// deleteFolder removes a server's folder. The data folder goes first and through a container
// when it has to, so a failure leaves profile.json behind and the server still listed.
func deleteFolder(ctx context.Context, manager runtime.Manager, dir, image string) error {
	data := filepath.Join(dir, "data")
	if err := os.RemoveAll(data); err != nil {
		if err := manager.DeleteAsRoot(ctx, data, image); err != nil {
			return err
		}
	}
	return os.RemoveAll(dir)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func find(profiles []config.Profile, id string) (config.Profile, bool) {
	for _, profile := range profiles {
		if profile.ID == config.Slug(id) {
			return profile, true
		}
	}
	return config.Profile{}, false
}
