package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Gryt-chat/cli/internal/app"
	"github.com/Gryt-chat/cli/internal/config"
	"github.com/Gryt-chat/cli/internal/doctor"
	"github.com/Gryt-chat/cli/internal/pull"
	gruntime "github.com/Gryt-chat/cli/internal/runtime"
	"github.com/Gryt-chat/cli/internal/updater"
)

var version = "dev"

func main() {
	root, err := config.DefaultRoot()
	if err != nil {
		fatal(err)
	}
	store := config.NewStore(root)
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "version", "--version", "-v":
			fmt.Println("gryt " + version)
			return
		case "help", "--help", "-h":
			printHelp()
			return
		case "update":
			os.Exit(runUpdate(args[1:]))
		case "pull":
			os.Exit(runPull(store, args[1:]))
		case "channel":
			os.Exit(runChannel(store, args[1:]))
		case "doctor":
			os.Exit(runDoctor(store, root))
		case "list":
			list(store)
			return
		case "env":
			if len(args) != 2 {
				fatal(fmt.Errorf("usage: gryt env <server-id>"))
			}
			showEnv(store, args[1])
			return
		default:
			fatal(fmt.Errorf("unknown command %q; run gryt help", args[0]))
		}
	}
	program := tea.NewProgram(app.New(store, gruntime.Docker{}, version))
	if _, err := program.Run(); err != nil {
		fatal(err)
	}
}

// runDoctor prints every check and returns the exit code, so a script can gate on it.

// runUpdate replaces this binary with the newest release, or with --check only reports
// whether there is one.
func runUpdate(args []string) int {
	checkOnly := len(args) > 0 && (args[0] == "--check" || args[0] == "check")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	release, err := updater.Check(ctx, http.DefaultClient)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gryt: could not reach the releases API:", err)
		return 1
	}

	if !updater.Newer(version, release.Tag) {
		fmt.Printf("gryt %s is the newest release.\n", version)
		return 0
	}

	if checkOnly {
		fmt.Printf("%s is available. You have %s. Run: gryt update\n", release.Tag, version)
		return 0
	}

	path, err := updater.Path()
	if err != nil {
		fatal(err)
	}
	fmt.Printf("Updating %s to %s\n", version, release.Tag)
	if err := updater.Apply(ctx, http.DefaultClient, release, path); err != nil {
		fmt.Fprintln(os.Stderr, "gryt:", err)
		fmt.Fprintln(os.Stderr, "If the binary is somewhere you cannot write, re-run the installer instead:")
		fmt.Fprintln(os.Stderr, "  curl -fsSL https://get.gryt.chat | sh")
		return 1
	}
	fmt.Printf("Updated to %s\n", release.Tag)
	return 0
}

// runPull moves one server onto the newest images. Named apart from gryt update, which
// replaces this binary and leaves every container where it is.
func runPull(store *config.Store, args []string) int {
	id := ""
	force, shared := false, false
	for _, arg := range args {
		switch {
		case arg == "--force" || arg == "-f":
			force = true
		case arg == "--shared":
			shared = true
		case strings.HasPrefix(arg, "-") || id != "":
			return pullUsage()
		default:
			id = arg
		}
	}
	if shared && id != "" {
		return pullUsage()
	}
	if !shared && id == "" {
		return pullUsage()
	}

	// Generous, because this is a registry pull over whatever connection the machine has,
	// and a server cut off partway through is worse than one that waits.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	beta := store.Preferences().IsBeta()
	latest := func(ctx context.Context) (string, error) {
		return updater.LatestServerRelease(ctx, http.DefaultClient, beta)
	}
	options := pull.Options{ServerID: id, Shared: shared, Force: force}
	if err := pull.Run(ctx, os.Stdout, store, gruntime.Docker{}, latest, options); err != nil {
		fmt.Fprintln(os.Stderr, "gryt:", err)
		return 1
	}
	return 0
}

func pullUsage() int {
	fmt.Fprintln(os.Stderr, "usage: gryt pull <server-id> [--force]  |  gryt pull --shared")
	return 1
}

// runChannel reads or sets the release channel this machine follows.
func runChannel(store *config.Store, args []string) int {
	if len(args) == 0 {
		fmt.Println(store.Preferences().Channel)
		return 0
	}
	switch args[0] {
	case config.ChannelStable, config.ChannelBeta:
		if err := store.SetChannel(args[0]); err != nil {
			fatal(err)
		}
		prefs := store.Preferences()
		fmt.Printf("Channel is now %s.\n", prefs.Channel)
		fmt.Printf("gryt update follows it, and servers you start run ghcr.io/gryt-chat/*:%s.\n", prefs.ImageTag())
		fmt.Println("Servers already here keep their image until you run gryt pull <server>.")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "gryt: channel must be %s or %s\n", config.ChannelStable, config.ChannelBeta)
		return 1
	}
}

func runDoctor(store *config.Store, root string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	profiles, err := store.List()
	if err != nil {
		fatal(err)
	}

	checks := doctor.Environment(ctx, doctor.Exec, root, profiles)
	for _, check := range checks {
		mark := "ok  "
		if !check.OK {
			mark = "FAIL"
		}
		fmt.Printf("%s  %-20s %s\n", mark, check.Name, check.Detail)
	}

	problems := doctor.Problems(checks)
	if len(problems) == 0 {
		return 0
	}
	fmt.Println()
	for _, problem := range problems {
		fmt.Printf("%s: %s\n", problem.Name, problem.Fix)
	}
	return 1
}

func list(store *config.Store) {
	profiles, err := store.List()
	if err != nil {
		fatal(err)
	}
	if len(profiles) == 0 {
		fmt.Println("No Gryt servers configured. Run gryt to create one.")
		return
	}
	for _, profile := range profiles {
		fmt.Printf("%-24s %-22s %s\n", profile.ID, profile.Address(), profile.Name)
	}
}

func showEnv(store *config.Store, id string) {
	profiles, err := store.List()
	if err != nil {
		fatal(err)
	}
	for _, profile := range profiles {
		if profile.ID != config.Slug(id) {
			continue
		}
		settings, err := store.Settings(profile)
		if err != nil {
			fatal(err)
		}
		for _, setting := range settings {
			value := setting.Value
			if setting.Sensitive {
				value = "••••••••"
			}
			fmt.Printf("%-32s %-10s %s\n", setting.Key, setting.Mode, value)
		}
		return
	}
	fatal(fmt.Errorf("server %q not found", id))
}

func printHelp() {
	fmt.Println(strings.TrimSpace(`gryt manages self-hosted Gryt Chat servers.

Usage:
  gryt                 Open the interactive server manager
  gryt channel         Print the release channel this machine follows
  gryt channel beta    Follow beta for this CLI and the servers it starts
  gryt update          Replace this binary with the newest release
  gryt update --check  Report whether a newer release exists, and change nothing
  gryt pull <server>   Pull the newest images for a server and recreate it
  gryt pull <server> --force
                       Pull without checking whether a newer release exists
  gryt pull --shared   Pull the voice server and object store every server here shares
  gryt doctor          Check Docker and the config directory, and say what to fix
  gryt list            List configured local servers
  gryt env <server>    Show settings and whether they are live or restart-bound
  gryt version         Print the CLI version

Configuration defaults to the operating system's user config directory.
Set GRYT_CONFIG_DIR to override it.`))
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "gryt:", err); os.Exit(1) }
