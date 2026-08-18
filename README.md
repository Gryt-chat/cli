# Gryt CLI

`gryt` is the terminal control plane for self-hosted Gryt Chat servers. Running
the command with no arguments opens a keyboard-first TUI for creating server
profiles, editing validated settings, inspecting health and logs, and managing
the generated Docker Compose deployment.

## Status

This repository contains the first usable foundation:

- Full-screen Bubble Tea v2 server workbench
- New-server and edit-server wizard
- Strict, Balanced, and Community security presets
- Validated bind address, port, voice capacity, proxy, SFU, and storage settings
- Private profile and `.env` storage in the operating system config directory
- Generated Docker Compose deployment using `ghcr.io/gryt-chat/server:latest`
- Start, stop, restart, health refresh, and recent logs
- Explicit live-versus-restart setting labels
- Plain `list` and `env` commands for scripts and inaccessible terminals

The CLI does not claim environment variables are hot-reloadable. Settings that
already live in the server SQLite database are labelled as live-ready; they
will be connected to a local authenticated management API in the next phase.

## Install

Requires Go 1.24 or newer for source installation:

```sh
go install github.com/Gryt-chat/cli/cmd/gryt@latest
gryt
```

Docker Desktop or Docker Engine with the Compose plugin is required to start a
generated deployment. Profile creation and `.env` generation work without it.

## Keyboard map

| Key | Action |
| --- | --- |
| `↑` / `↓` or `k` / `j` | Select server |
| `n` | New server wizard |
| `e` | Edit selected server |
| `s` | Start |
| `x` | Stop |
| `r` | Restart |
| `l` | Recent logs |
| `g` | Refresh health |
| `q` | Quit |

The wizard uses `Enter` to advance/save, `Shift+Tab` to move back, arrow keys to
change a choice, and `Esc` to cancel.

## Files

By default, profiles live below the platform user config directory:

```text
gryt/
└── servers/
    └── my-server/
        ├── profile.json
        ├── .env
        ├── compose.yaml
        └── data/
```

Set `GRYT_CONFIG_DIR` to use another root. Profile directories and files are
created with private permissions where the operating system supports them.

## Runtime settings roadmap

The server already stores its name, description, discovery flag, join policy,
LAN-open mode, upload limits, profanity settings, and channels in SQLite. The
next server change should add a local authenticated management endpoint around
that model and introduce a persisted connection gate. See
[`docs/runtime-settings.md`](docs/runtime-settings.md) for the proposed boundary.

## Development

```sh
go test ./...
go vet ./...
go run ./cmd/gryt
```

## Sponsors

What sponsoring pays for, the tiers, and everyone who has sponsored:
[gryt.chat/sponsors](https://gryt.chat/sponsors). To sponsor:
[GitHub Sponsors](https://github.com/sponsors/Gryt-chat).

The list itself lives in the [Gryt README](https://github.com/Gryt-chat/gryt#sponsors),
in one place rather than ten, so it cannot fall out of step across repositories.

## License

AGPL-3.0-or-later, matching the Gryt project.
