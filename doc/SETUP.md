# Setup and operation

This bot runs as a long-lived Docker service on an always-on Linux machine.
Discord role members can use `/start` to wake and start the Minecraft Java
server and `/status` to check it. After Minecraft has remained empty for 15
minutes, the bot sends RCON `stop`. If Minecraft then remains stopped for
another 15 minutes without a new start request, a host-side systemd timer powers
off the Minecraft host.

## Requirements

- An always-on Linux Docker host on the same LAN as the Minecraft host. Do not
  run the bot on the host that it powers off.
- A Minecraft Java Edition server with RCON enabled.
- A systemd service for Minecraft on the Minecraft host.
- A Discord application and bot, installed in the target Discord server with
  the `bot` and `applications.commands` OAuth scopes.
- Wake-on-LAN enabled in the Minecraft host's firmware and network adapter.
- SSH access from the Docker host to the Minecraft host, using the dedicated
  restricted key described below.

The container uses host networking because LAN broadcast packets generally do
not pass through Docker's default bridged network. Configure the host firewall
to allow the bot to reach the Minecraft RCON port and send UDP broadcasts.

## Configure Minecraft

In the server's `server.properties`, configure RCON with a long, unique
password:

```properties
enable-rcon=true
rcon.port=25575
rcon.password=replace-with-a-long-random-password
```

Restart Minecraft after changing these settings. Restrict the RCON port in the
host firewall to the Docker host or LAN as appropriate; do not expose RCON to
the public internet. The bot uses RCON for both the Java Edition `list` and
`stop` commands.

Install a Minecraft systemd service on the target Linux host. Start with
[`docker/host/minecraft.service.example`](../docker/host/minecraft.service.example)
and edit its Minecraft user, directory, memory, and JAR settings. Install it as
`/etc/systemd/system/minecraft.service` (or use a different unit name and set
that name in `/etc/default/minecraft-discord-bot` below). The bot starts this
unit over SSH when `/start` is used while the host is already powered on.
Create the configured Linux user and give it write access to the Minecraft
working directory.

For example, after editing the template:

```sh
sudo install -o root -g root -m 0644 /path/to/minecraft.service \
  /etc/systemd/system/minecraft.service
sudo systemctl daemon-reload
```

The bot starts this unit on demand. To start Minecraft automatically on every
host boot, enable it with `sudo systemctl enable minecraft.service`.

Configure Wake-on-LAN in the server's firmware and network adapter settings.
Use the MAC address of the adapter that will receive the magic packet. If the
LAN does not route `255.255.255.255` broadcasts to that adapter, set
`WOL_BROADCAST` to the subnet's directed broadcast address (for example,
`192.168.1.255` on a `/24` network).

## Configure restricted host control

The bot needs SSH to start Minecraft and manage the delayed host-shutdown timer.
It does not get a shell or a general-purpose root key: the SSH key is restricted
to an allowlisted helper, and `sudo` permits only that helper.

On the Minecraft host:

1. Create the dedicated SSH account and install the helper scripts and systemd
   units from `docker/host/`:

   ```sh
   sudo useradd --create-home --shell /bin/sh minecraft-bot
   sudo install -o root -g root -m 0755 docker/host/minecraft-discord-control \
     /usr/local/sbin/minecraft-discord-control
   sudo install -o root -g root -m 0755 docker/host/minecraft-discord-control-root \
     /usr/local/sbin/minecraft-discord-control-root
   sudo install -o root -g root -m 0755 docker/host/minecraft-host-poweroff \
     /usr/local/sbin/minecraft-host-poweroff
   sudo install -o root -g root -m 0644 docker/host/minecraft-host-shutdown.service \
     /etc/systemd/system/minecraft-host-shutdown.service
   sudo install -o root -g root -m 0644 docker/host/minecraft-host-shutdown.timer \
     /etc/systemd/system/minecraft-host-shutdown.timer
   ```

2. Set the Minecraft service name in the root-owned configuration file:

   ```sh
   echo 'MINECRAFT_SYSTEMD_UNIT=minecraft.service' |
     sudo tee /etc/default/minecraft-discord-bot
   sudo chmod 0644 /etc/default/minecraft-discord-bot
   ```

   If the Minecraft unit has another name, use it here. The same file is read
   by the restricted helper and by the power-off check.

3. Allow the SSH account to run only the root helper:

   ```sh
   sudo visudo -f /etc/sudoers.d/minecraft-discord-bot
   ```

   Add this line:

   ```text
   minecraft-bot ALL=(root) NOPASSWD: /usr/local/sbin/minecraft-discord-control-root start-minecraft, /usr/local/sbin/minecraft-discord-control-root schedule-shutdown, /usr/local/sbin/minecraft-discord-control-root cancel-shutdown
   ```

   Validate the sudoers file with `sudo visudo -c`.

4. On the Docker host, create a dedicated key pair and obtain the Minecraft
   host's SSH host key:

   ```sh
   mkdir -p secrets
   ssh-keygen -t ed25519 -f secrets/minecraft_bot_ed25519 -N ''
   ssh-keyscan -p 22 192.168.1.50 > secrets/known_hosts
   ```

   Verify the host key fingerprint through a trusted channel before using the
   `known_hosts` file. `ssh-keyscan` alone does not authenticate a host. The
   private key must be readable by the container's UID `10001`, but by no
   untrusted host users. Use a dedicated unencrypted key for unattended startup:

   ```sh
   sudo chown 10001:10001 secrets/minecraft_bot_ed25519
   chmod 0755 secrets
   chmod 0600 secrets/minecraft_bot_ed25519
   chmod 0644 secrets/known_hosts
   ```

5. Add the public key from `secrets/minecraft_bot_ed25519.pub` to
   `/home/minecraft-bot/.ssh/authorized_keys` on the Minecraft host, prefixed
   with the forced command and restrictions below. Replace the key text with
   the contents of the `.pub` file:

   ```text
   restrict,command="/usr/local/sbin/minecraft-discord-control" ssh-ed25519 AAAA... minecraft-discord-bot
   ```

   Make sure the account's `.ssh` directory and `authorized_keys` are owned by
   `minecraft-bot` and are not writable by other users. Restrict SSH ingress to
   the Docker host with the host firewall.

After installing the unit and helper files, reload systemd:

```sh
sudo systemctl daemon-reload
```

The helper accepts only `start-minecraft`, `schedule-shutdown`, and
`cancel-shutdown`. It cancels any pending timer before starting Minecraft; the
timer's power-off service also verifies that the Minecraft systemd unit is
still inactive before powering off. Keep the bot's Docker host separate and
always on so it can wake the Minecraft host later.

## Configure Discord

1. Create an application in the Discord Developer Portal and add a bot.
2. Copy its bot token; treat it as a secret.
3. Copy the application's ID from the Developer Portal.
4. Invite it to the target server with the `bot` and `applications.commands`
   scopes. It does not need administrator permissions.
5. Enable Developer Mode in Discord and copy the target server ID and each
   role ID that should be allowed to use the bot.

The bot creates guild-scoped `/start` and `/status` commands at startup. Both
commands are restricted by the configured role IDs; unauthorized uses receive
an ephemeral denial. Configure role IDs as a comma-separated list.

## Configure and start the container

From the repository root:

```sh
cp .env.example .env
chmod 0600 .env
```

Edit `.env` and provide the Discord token, application, guild, and allowed role
IDs, the server adapter's MAC address, the RCON address/password, the host SSH
address and user, and any necessary broadcast address or port.
`MINECRAFT_RCON_ADDRESS` and `HOST_SSH_ADDRESS` must be reachable `host:port`
addresses, such as `192.168.1.50:25575` and `192.168.1.50:22`.
The Compose file mounts `./secrets` read-only at `/run/secrets`; create this
directory and place the private key and verified `known_hosts` file there.
`STARTUP_TIMEOUT` defaults to five minutes and accepts a Go duration such as
`7m`. It bounds the total `/start` wait for the host and Minecraft to become
available.

Then build and start the service:

```sh
docker compose up --build -d
docker compose logs -f minecraft-discord-bot
```

Use `docker compose restart` to restart the bot and `docker compose down` to
stop it. The Compose service restarts automatically unless explicitly stopped.
The `.env` file is excluded from version control; do not commit tokens or RCON
passwords.

## Behavior and recovery

- `/start` first tries the restricted SSH helper to cancel the power-off timer
  and start the Minecraft systemd service. If SSH is unavailable, it sends a
  magic packet and retries SSH and RCON until Minecraft responds or
  `STARTUP_TIMEOUT` expires. An already reachable server is left running.
- `/status` checks RCON without waking the host. A failed check is reported as
  unavailable because the host may be off, starting, or temporarily unreachable.
- The monitor checks RCON every 30 seconds. It starts the 15-minute idle
  countdown only after a successful player-list response reports zero players.
  A positive player count resets the countdown.
- When RCON is unavailable but the Linux host is reachable over SSH, the bot
  schedules a host power-off 15 minutes later. `/start` cancels that timer and
  starts Minecraft again. The timer is managed by systemd on the host, so it
  survives a bot restart.
- The host power-off unit checks that the configured Minecraft systemd service
  is still inactive before issuing `systemctl poweroff`. If the service is
  activating or running when the timer expires, the host remains on.
- The Minecraft-empty countdown is held in memory and restarts from a fresh
  15-minute grace period after a bot restart. The second-stage host timer is
  managed by systemd on the Minecraft host and survives bot restarts.
- If RCON becomes unreachable, the bot resets the countdown and does not issue
  a shutdown based on stale player data. After RCON recovers with zero players,
  a fresh 15-minute countdown begins.
- The bot logs operational events and errors as JSON to standard output. Check
  `docker compose logs` if startup, RCON, or Wake-on-LAN is not working.

The bot is intended to send Wake-on-LAN from a Linux Docker host on the server's
LAN. Some routers, VLANs, Wi-Fi adapters, and host firewalls block broadcasts;
verify packet delivery and BIOS/network-adapter wake settings if `/start` times
out.

## Configuration reference

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `DISCORD_TOKEN` | Yes | — | Discord bot token |
| `DISCORD_APPLICATION_ID` | Yes | — | Discord application ID |
| `DISCORD_GUILD_ID` | Yes | — | Discord server where commands are registered |
| `DISCORD_ALLOWED_ROLE_IDS` | Yes | — | Comma-separated role IDs permitted to run commands |
| `WOL_MAC` | Yes | — | Six-byte Ethernet MAC address of the Minecraft host |
| `WOL_BROADCAST` | No | `255.255.255.255` | IPv4 broadcast destination |
| `WOL_PORT` | No | `9` | UDP destination port for the magic packet |
| `MINECRAFT_RCON_ADDRESS` | Yes | — | Minecraft RCON `host:port` |
| `MINECRAFT_RCON_PASSWORD` | Yes | — | Minecraft RCON password |
| `HOST_SSH_ADDRESS` | Yes | — | Minecraft host SSH `host:port` |
| `HOST_SSH_USER` | Yes | — | Restricted SSH account |
| `HOST_SSH_KEY_FILE` | Yes | — | Private key path inside the container |
| `HOST_SSH_KNOWN_HOSTS_FILE` | Yes | — | Pinned host keys path inside the container |
| `STARTUP_TIMEOUT` | No | `5m` | Maximum total wait for host and Minecraft on `/start` |
