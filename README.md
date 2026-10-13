# Minecraft Wake-on-LAN Discord Bot

A self-hosted Discord bot that lets authorized users start and monitor a
Minecraft Java server. It wakes the Linux Minecraft host over Wake-on-LAN,
starts Minecraft through a restricted SSH command, and shuts down Minecraft
after 15 minutes with no players. If Minecraft stays stopped for another
15 minutes, a host-side systemd timer powers off the Minecraft machine.

The bot must run in Docker on a **separate, always-on Linux host** on the same
LAN. The Minecraft host needs Java Edition RCON, SSH, systemd, and Wake-on-LAN
configured. Read the full [setup and operations guide](doc/SETUP.md) before
deploying; powering off a physical host is intentionally an explicit,
host-specific setup step.

## Features

- Discord slash commands: `/start` and `/status`.
- Role-based command authorization within one configured Discord server.
- Wake-on-LAN magic packet generation and startup polling.
- Minecraft Java RCON player monitoring and graceful `stop`.
- A 15-minute empty-server delay, followed by a separate 15-minute host
  shutdown delay.
- SSH host-key verification and a restricted host-side command allowlist.
- Docker Compose deployment with automatic container restart and JSON logs.

## Quick start

1. Follow the [full setup guide](doc/SETUP.md) to configure Minecraft RCON,
   Wake-on-LAN, a systemd service, restricted SSH access, and the host power-off
   timer.
2. Create the bot in the Discord Developer Portal and invite it to your server
   with the `bot` and `applications.commands` scopes.
3. Copy `.env.example` to `.env`, protect it with `chmod 0600 .env`, and set
   every required Discord, RCON, Wake-on-LAN, and SSH value.
4. Put the dedicated SSH private key and verified `known_hosts` file in the
   ignored `secrets/` directory. See the guide for required ownership and
   permissions.
5. Build and start the bot:

   ```sh
   docker compose up --build -d
   docker compose logs -f minecraft-discord-bot
   ```

The Compose service uses host networking for LAN broadcast delivery. Do not
expose the RCON port or SSH key to the public internet. The bot needs network
access to Discord, the Minecraft RCON port, and the Minecraft host's SSH port.

## Commands and timing

| Command | Behavior |
| --- | --- |
| `/start` | If RCON is already reachable, reports that Minecraft is online. Otherwise tries the restricted SSH start command; if the host is unavailable, sends Wake-on-LAN, retries SSH and RCON, and reports when Minecraft is ready or the configured startup timeout expires. |
| `/status` | Checks RCON and reports the current player count. It does not wake the host. |

Only members with one of the configured `DISCORD_ALLOWED_ROLE_IDS` can run
commands. Responses are ephemeral.

The idle countdown begins after a successful RCON `list` reports zero players.
Any players reset it. After 15 minutes continuously empty, the bot rechecks the
player count and sends the Java `stop` command. After RCON becomes unreachable,
the host-side timer waits another 15 minutes, then checks that the Minecraft
systemd unit is inactive before powering off the host. `/start` cancels the
timer and starts Minecraft again.

## Development checks

With the Go version declared in [`go.mod`](go.mod) installed:

```sh
go test -race ./...
go vet ./...
go build -o /tmp/minecraft-discord-bot ./src
```

## Repository layout

- [`src/`](src/) — Go application and internal packages.
- [`docker/`](docker/) — Container image definition and Linux host helper and
  systemd templates.
- [`doc/SETUP.md`](doc/SETUP.md) — Detailed prerequisites, setup, operations,
  security notes, and troubleshooting.
- [`.env.example`](.env.example) — Documented environment-variable template.
- [`docker-compose.yml`](docker-compose.yml) — Local Compose deployment.

## License

See [LICENSE](LICENSE).
