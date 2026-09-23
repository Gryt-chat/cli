// Package pull moves containers onto the images their release channel points at. Separate
// from internal/updater, which replaces the CLI binary and touches no container.
package pull

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Gryt-chat/cli/internal/config"
	"github.com/Gryt-chat/cli/internal/runtime"
	"github.com/Gryt-chat/cli/internal/updater"
)

// Latest reports the newest published server release. A parameter so the tests run without
// a network, and so the check has one place to live.
type Latest func(context.Context) (string, error)

type Options struct {
	ServerID string
	// Moves the project holding the voice server and the object store instead of one
	// server. Its own switch because recreating the SFU drops calls on every server here.
	Shared bool
	// Skips the release check and pulls whatever the channel's tag points at now. The way
	// through when GitHub cannot be reached, or when the tag moved without a new release.
	Force bool
}

// Run pulls images and recreates whatever changed. Everything is fetched before any
// container is replaced, so a pull that dies partway leaves what is running alone.
func Run(ctx context.Context, out io.Writer, store *config.Store, manager runtime.Manager, latest Latest, opts Options) error {
	if err := manager.Available(ctx); err != nil {
		return err
	}
	if opts.Shared {
		return runShared(ctx, out, store, manager)
	}
	return runServer(ctx, out, store, manager, latest, opts)
}

func runServer(ctx context.Context, out io.Writer, store *config.Store, manager runtime.Manager, latest Latest, opts Options) error {
	profiles, err := store.List()
	if err != nil {
		return err
	}
	profile, ok := find(profiles, opts.ServerID)
	if !ok {
		return fmt.Errorf("server %q not found. Run gryt list to see the servers on this machine", opts.ServerID)
	}

	container := "gryt-" + profile.ID
	if !manager.ContainerRunning(ctx, container) {
		return fmt.Errorf("%s is not running, so there is nothing to recreate. Start it from gryt and run this again", profile.Name)
	}

	before := manager.ContainerEnv(ctx, container, "SERVER_VERSION")
	tag := store.Preferences().ImageTag()
	ref := manager.ContainerImageRef(ctx, container)
	// A server started before gryt channel beta runs the stable tag, and the newest release
	// is the same number on both. Without this it would be told there is nothing to pull.
	offChannel := ref != "" && ref != "ghcr.io/gryt-chat/server:"+tag

	switch {
	case opts.Force:
		fmt.Fprintf(out, "Pulling the %s images for %s without checking for a newer release.\n", tag, profile.Name)
	case offChannel:
		fmt.Fprintf(out, "%s runs %s, and this machine's channel points at %s. Pulling.\n", profile.Name, ref, tag)
	case before == "":
		fmt.Fprintf(out, "%s reports no version, so there is no telling whether it is behind. Pulling.\n", profile.Name)
	default:
		newest, err := latest(ctx)
		if err != nil {
			return fmt.Errorf("could not ask GitHub which server release is newest: %v. Run gryt pull %s --force to pull without checking", err, profile.ID)
		}
		if !updater.Newer(before, newest) {
			fmt.Fprintf(out, "%s runs %s, which is the newest server release. Nothing to pull.\n", profile.Name, before)
			fmt.Fprintf(out, "Run gryt pull %s --force to pull the %s images anyway.\n", profile.ID, tag)
			return nil
		}
		fmt.Fprintf(out, "%s runs %s. The newest release is %s.\n", profile.Name, before, strings.TrimPrefix(newest, "v"))
	}

	// Rewritten first so the pull follows the channel this machine is on now. A server made
	// before gryt channel beta still names the stable tag in the compose file it was given.
	dir := store.ServerDir(profile.ID)
	if _, err := store.WriteCompose(profile); err != nil {
		return err
	}

	// An image worker only exists when the server uses this machine's object store, and
	// naming one that is not there reads as a pull that quietly skipped something.
	fmt.Fprintf(out, "\nPulling the %s %s.\n", tag, what(profile))
	if err := manager.Pull(ctx, profile, dir, out); err != nil {
		fmt.Fprintf(out, "\nNothing was recreated. %s is still running %s.\n", profile.Name, describe(before))
		fmt.Fprintf(out, "Run gryt pull %s again once the registry answers.\n", profile.ID)
		return err
	}

	fmt.Fprintf(out, "\nRecreating %s.\n", profile.Name)
	if err := manager.Start(ctx, profile, dir); err != nil {
		fmt.Fprintf(out, "\nThe images are on this machine, but %s did not come back up and may be stopped.\n", profile.Name)
		fmt.Fprintf(out, "Run gryt pull %s again to retry it. The images are here already, so it will be quick.\n", profile.ID)
		return err
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, versions(profile.Name, tag, before, manager.ContainerEnv(ctx, container, "SERVER_VERSION")))
	// The SFU is a compose project of its own, shared by every server on the machine, so
	// moving it is not something one server's update gets to do on its own.
	if manager.ContainerRunning(ctx, config.SFUContainer) {
		fmt.Fprintln(out, "The voice server is shared by every server here and was left alone. gryt pull --shared moves it.")
	}
	return nil
}

// runShared moves the voice server and the object store. Recreating them cuts voice and
// uploads for every server on the machine, which is why it is asked for separately.
func runShared(ctx context.Context, out io.Writer, store *config.Store, manager runtime.Manager) error {
	if !manager.ContainerRunning(ctx, config.SFUContainer) {
		return fmt.Errorf("the shared voice server is not running, so there is nothing to recreate. Start it from gryt and run this again")
	}
	dir := store.SharedDir()
	if _, err := store.WriteSharedCompose(); err != nil {
		return err
	}
	before := manager.ContainerImageID(ctx, config.SFUContainer)

	fmt.Fprintf(out, "Pulling the %s images for the voice server and the object store.\n", store.Preferences().ImageTag())
	fmt.Fprintln(out, "Anything that has moved is recreated, which interrupts voice on every server here.")
	if err := manager.PullShared(ctx, dir, out); err != nil {
		fmt.Fprintln(out, "\nNothing was recreated. Voice and uploads are still up on the images they had.")
		fmt.Fprintln(out, "Run gryt pull --shared again once the registry answers.")
		return err
	}

	fmt.Fprintln(out, "\nRecreating the shared services whose images moved.")
	if err := manager.EnsureShared(ctx, dir); err != nil {
		fmt.Fprintln(out, "\nThe shared services did not come back. Voice and uploads are down for every server here.")
		fmt.Fprintln(out, "Run gryt pull --shared again to retry it. The images are here already, so it will be quick.")
		return err
	}

	after := manager.ContainerImageID(ctx, config.SFUContainer)
	fmt.Fprintln(out)
	if before == after {
		fmt.Fprintln(out, "The shared services are up. Their images had not moved, so nothing was replaced.")
		return nil
	}
	fmt.Fprintf(out, "The voice server is on a new image: %s → %s.\n", short(before), short(after))
	return nil
}

// versions is the line to read to see whether it worked: what the server was on, and what
// it is on now.
func versions(name, tag, before, after string) string {
	switch {
	case after == "":
		return fmt.Sprintf("%s is back up. Its image reports no version, so there is nothing to compare.", name)
	case before == after:
		return fmt.Sprintf("%s is back up on the %s image and still reports %s.", name, tag, after)
	case before == "":
		return fmt.Sprintf("%s now runs %s.", name, after)
	default:
		return fmt.Sprintf("%s: %s → %s.", name, before, after)
	}
}

// what names the images about to be pulled, which is one or two depending on the server.
func what(profile config.Profile) string {
	if profile.StorageBackend == config.SharedStorage {
		return "images for the server and its image worker"
	}
	return "image for the server"
}

func describe(version string) string {
	if version == "" {
		return "the image it started on"
	}
	return version
}

// short cuts an image id down to the twelve characters docker itself prints.
func short(image string) string {
	image = strings.TrimPrefix(image, "sha256:")
	if len(image) > 12 {
		return image[:12]
	}
	return image
}

func find(profiles []config.Profile, id string) (config.Profile, bool) {
	for _, profile := range profiles {
		if profile.ID == config.Slug(id) {
			return profile, true
		}
	}
	return config.Profile{}, false
}
