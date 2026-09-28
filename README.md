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
checksums, and installs to `/usr/local/bin` if that's writable or
`~/.local/bin` if not. `GRYT_VERSION` installs a specific tag instead of the
newest release, and `GRYT_INSTALL_DIR` changes where the binary lands.

It doesn't cover Windows. Download the `.zip` from the
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
| `enter` | One server, with its addresses grouped by who they're for |
| `s` | Start |
| `x` | Stop |
| `r` | Restart |
| `l` | Recent logs |
| `D` | Remove the server, once you've typed its id |
| `g` | Refresh health |
| `u` | Update, when one is available |
| `q` | Quit |

The wizard uses `Enter` to advance/save, `Shift+Tab` to move back, arrow keys to
change a choice, and `Esc` to cancel.

## Creating and starting a server from a script

```sh
gryt create --name my-server --port 5000 --yes
gryt start my-server
```

`gryt create` takes every flag the wizard asks for, and previews what it would
create without writing anything until `--yes` is added:

| Flag | Same as | Default |
| --- | --- | --- |
| `--name` | Server name | required |
| `--host` | Bind address | `0.0.0.0` |
| `--port` | Port | the first port nothing else on the machine holds |
| `--security` | Security level (`strict`, `balanced` or `community`) | `balanced` |
| `--voice-seats` | Voice seats | `0` (no limit) |
| `--proxy-hops` | Trusted proxy hops | `0` |
| `--domain` | The typed address in "Where will people connect from?" | none |
| `--lan` | Ticking this machine's own addresses in the same question | off |

`--domain` takes a `ws://` or `wss://` address, and can be repeated or given as
a comma-separated list. Without `--domain` or `--lan`, the server is only
reachable at `ws://localhost:5005` — the same as leaving every box but "This
machine only" unticked in the wizard.

`gryt start <server>` brings up the shared voice server, then the named one.
It's the same two steps behind the manager's `s` key, and it exits non-zero if
Docker isn't available or the server was never created.

Both exit `0` on success and non-zero otherwise, so a script can chain them
with `gryt pull` and `gryt remove` without driving a terminal.

## Files

By default, profiles live below the platform user config directory:

```text
gryt/
└── servers/
    └── my-server/
        ├── profile.json
        ├── .env
        ├── admin.env
        ├── compose.yaml
        └── data/
```

Set `GRYT_CONFIG_DIR` to use another root. Profile directories and files are
created with private permissions where the operating system supports them.

`admin.env` holds the management token, apart from `.env` so that one stays safe
to paste into a report. On Linux, `data/` belongs to uid 1001, the user the
server runs as. A `data-init` container hands it over each time the server
starts.

## Changing a running server

Some of a server's settings live in its database rather than its environment:
who may join, whether it advertises itself over mDNS, whether the LAN is open,
and what it does about profanity. `c` changes those on a running server, through
the local management API the server publishes on loopback.

That needs server 1.5.0 or newer. Against anything older the screen says the
server has no management API and points at `gryt pull`.

Everything else lives in the generated `.env` and takes effect on restart. The
settings screen and `gryt env` both label which is which.

## Moving a server onto a newer image

```sh
gryt pull my-server
```

That pulls the image the release channel points at, recreates the server and
its image worker, and prints the version it was on next to the version it's on
now. It stops before doing anything when the server isn't running, or when the
server already runs the newest release. `--force` pulls without checking.

Restarting never pulled anything. `docker compose restart` reruns the container
you already have, so the manager's version line names this command instead.

The voice server is a compose project of its own, shared by every server on
the machine. `gryt pull --shared` moves it. Uploads live in each server's own
data folder. Servers set up before that still keep theirs in a MinIO in the
same shared project, and it stays for as long as one of them uses it.

### Checking automatically instead of by hand

```sh
gryt pull --auto on
```

Installs the same nightly systemd timer Compose self-hosters use
(`ops/deploy/auto-update` in the [gryt repo](https://github.com/Gryt-chat/gryt)),
pointed at the shared voice server and every server this machine runs instead
of `gryt-<stack>-<service>`. `gryt pull --auto off` removes it, and `gryt pull
--auto status` says whether it's running. Re-run `on` after adding a server,
because it writes the container list each time. Linux with systemd only, and it
asks for `sudo` to install the unit files.

## Removing a server

```sh
gryt remove my-server
```

Says what goes, asks you to type the server's id, then takes its containers
down and deletes its folder, database and uploads included. `--yes` skips the
question. The last server on the machine takes the voice server, the `gryt`
network and `shared/` with it. The images stay, and it prints the command that
removes them.

## Development

```sh
go test ./...
go vet ./...
go run ./cmd/gryt
```

## Issues

Please report bugs and request features in the
[main Gryt repository](https://github.com/Gryt-chat/gryt/issues).

## Sponsors

What sponsoring pays for, the tiers, and everyone who has sponsored:
[gryt.chat/sponsors](https://gryt.chat/sponsors). To sponsor:
[GitHub Sponsors](https://github.com/sponsors/Gryt-chat).

The list itself lives in the [Gryt README](https://github.com/Gryt-chat/gryt#sponsors),
in one place rather than ten, so it cannot fall out of step across repositories.

## License

[AGPL-3.0](https://github.com/Gryt-chat/gryt/blob/main/LICENSE) — Part of [Gryt](https://github.com/Gryt-chat/gryt)
