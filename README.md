<div align="center">
  <img src="https://raw.githubusercontent.com/Gryt-chat/client/main/public/logo.svg" width="80" alt="Gryt logo" />
  <h1>Gryt CLI</h1>
  <p>The terminal manager for self-hosted <a href="https://github.com/Gryt-chat/gryt">Gryt</a> servers.<br />Creates a server with working voice and uploads, then starts, stops, configures and updates every server on the machine.</p>
</div>

<br />

## Install

```sh
curl -fsSL https://get.gryt.chat | sh
```

The script picks the build for your platform, checks it against the release
checksums, and installs to `/usr/local/bin` if that is writable or
`~/.local/bin` if not. `GRYT_VERSION` installs a specific tag instead of the
newest release, and `GRYT_INSTALL_DIR` changes where the binary lands.

It does not cover Windows. Download the `.zip` from the
[releases page](https://github.com/Gryt-chat/cli/releases) instead.

From source, with Go 1.25 or newer:

```sh
go install github.com/Gryt-chat/cli/cmd/gryt@latest
```

Docker Desktop or Docker Engine with the Compose plugin is required to start a
generated deployment. Profile creation and `.env` generation work without it.

Full documentation: [docs.gryt.chat/docs/cli](https://docs.gryt.chat/docs/cli).

## Keyboard map

| Key | Action |
| --- | --- |
| `↑` / `↓` or `k` / `j` | Select server |
| `n` | New server wizard |
| `e` | Edit selected server |
| `c` | Change the settings the server keeps in its own database |
| `enter` | One server, with its addresses grouped by who they are for |
| `s` | Start |
| `x` | Stop |
| `r` | Restart |
| `l` | Recent logs |
| `g` | Refresh health |
| `u` | Update, when one is available |
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

## Changing a running server

Some of a server's settings live in its database rather than its environment:
who may join, whether it advertises itself over mDNS, whether the LAN is open,
and what it does about profanity. `c` changes those on a running server, through
the local management API the server publishes on loopback.

That needs server 1.5.0 or newer. Against anything older the screen says the
server has no management API and tells you to update its image and restart it.

Everything else lives in the generated `.env` and takes effect on restart. The
settings screen and `gryt env` both label which is which.

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

[AGPL-3.0](https://github.com/Gryt-chat/gryt/blob/main/LICENSE) — Part of [Gryt](https://github.com/Gryt-chat/gryt)
