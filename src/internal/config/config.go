package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultWOLBroadcast = "255.255.255.255"
	defaultWOLPort      = 9
	defaultStartupLimit = 5 * time.Minute
)

type Config struct {
	DiscordToken   string
	ApplicationID  string
	GuildID        string
	AllowedRoleIDs []string
	WOLMAC         net.HardwareAddr
	WOLBroadcast   net.IP
	WOLPort        int
	RCONAddress    string
	RCONPassword   string
	HostSSHAddress string
	HostSSHUser    string
	HostSSHKeyFile string
	KnownHostsFile string
	StartupTimeout time.Duration
}

func Load() (Config, error) {
	var cfg Config
	var err error

	if cfg.DiscordToken, err = required("DISCORD_TOKEN"); err != nil {
		return cfg, err
	}
	if cfg.ApplicationID, err = required("DISCORD_APPLICATION_ID"); err != nil {
		return cfg, err
	}
	if !isSnowflake(cfg.ApplicationID) {
		return cfg, fmt.Errorf("DISCORD_APPLICATION_ID must be a numeric Discord ID")
	}
	if cfg.GuildID, err = required("DISCORD_GUILD_ID"); err != nil {
		return cfg, err
	}
	if !isSnowflake(cfg.GuildID) {
		return cfg, fmt.Errorf("DISCORD_GUILD_ID must be a numeric Discord ID")
	}

	roles, err := required("DISCORD_ALLOWED_ROLE_IDS")
	if err != nil {
		return cfg, err
	}
	for _, role := range strings.Split(roles, ",") {
		role = strings.TrimSpace(role)
		if !isSnowflake(role) {
			return cfg, fmt.Errorf("DISCORD_ALLOWED_ROLE_IDS must be a comma-separated list of numeric Discord role IDs")
		}
		cfg.AllowedRoleIDs = append(cfg.AllowedRoleIDs, role)
	}

	mac, err := required("WOL_MAC")
	if err != nil {
		return cfg, err
	}
	cfg.WOLMAC, err = net.ParseMAC(mac)
	if err != nil || len(cfg.WOLMAC) != 6 {
		return cfg, fmt.Errorf("WOL_MAC must be a valid 6-byte Ethernet MAC address")
	}

	broadcast := valueOrDefault("WOL_BROADCAST", defaultWOLBroadcast)
	cfg.WOLBroadcast = net.ParseIP(broadcast).To4()
	if cfg.WOLBroadcast == nil {
		return cfg, fmt.Errorf("WOL_BROADCAST must be an IPv4 broadcast address")
	}

	port, err := strconv.Atoi(valueOrDefault("WOL_PORT", strconv.Itoa(defaultWOLPort)))
	if err != nil || port < 1 || port > 65535 {
		return cfg, fmt.Errorf("WOL_PORT must be an integer between 1 and 65535")
	}
	cfg.WOLPort = port

	if cfg.RCONAddress, err = required("MINECRAFT_RCON_ADDRESS"); err != nil {
		return cfg, err
	}
	host, portText, err := net.SplitHostPort(cfg.RCONAddress)
	if err != nil || host == "" {
		return cfg, fmt.Errorf("MINECRAFT_RCON_ADDRESS must be a host:port address")
	}
	rconPort, err := strconv.Atoi(portText)
	if err != nil || rconPort < 1 || rconPort > 65535 {
		return cfg, fmt.Errorf("MINECRAFT_RCON_ADDRESS must contain a port between 1 and 65535")
	}
	if cfg.RCONPassword, err = required("MINECRAFT_RCON_PASSWORD"); err != nil {
		return cfg, err
	}

	if cfg.HostSSHAddress, err = required("HOST_SSH_ADDRESS"); err != nil {
		return cfg, err
	}
	host, portText, err = net.SplitHostPort(cfg.HostSSHAddress)
	if err != nil || host == "" {
		return cfg, fmt.Errorf("HOST_SSH_ADDRESS must be a host:port address")
	}
	sshPort, err := strconv.Atoi(portText)
	if err != nil || sshPort < 1 || sshPort > 65535 {
		return cfg, fmt.Errorf("HOST_SSH_ADDRESS must contain a port between 1 and 65535")
	}
	if cfg.HostSSHUser, err = required("HOST_SSH_USER"); err != nil {
		return cfg, err
	}
	if cfg.HostSSHKeyFile, err = required("HOST_SSH_KEY_FILE"); err != nil {
		return cfg, err
	}
	if cfg.KnownHostsFile, err = required("HOST_SSH_KNOWN_HOSTS_FILE"); err != nil {
		return cfg, err
	}

	cfg.StartupTimeout, err = time.ParseDuration(valueOrDefault("STARTUP_TIMEOUT", defaultStartupLimit.String()))
	if err != nil || cfg.StartupTimeout <= 0 {
		return cfg, fmt.Errorf("STARTUP_TIMEOUT must be a positive Go duration, such as 5m")
	}
	return cfg, nil
}

func required(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func valueOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func isSnowflake(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
