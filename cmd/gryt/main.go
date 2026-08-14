package main

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/Gryt-chat/cli/internal/app"
	"github.com/Gryt-chat/cli/internal/config"
	gruntime "github.com/Gryt-chat/cli/internal/runtime"
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
	program := tea.NewProgram(app.New(store, gruntime.Docker{}))
	if _, err := program.Run(); err != nil {
		fatal(err)
	}
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
		for _, setting := range profile.EnvSettings() {
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
  gryt list            List configured local servers
  gryt env <server>    Show settings and whether they are live or restart-bound
  gryt version         Print the CLI version

Configuration defaults to the operating system's user config directory.
Set GRYT_CONFIG_DIR to override it.`))
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "gryt:", err); os.Exit(1) }
