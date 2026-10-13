package config

import (
	"testing"
	"time"
)

func setValidEnvironment(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"DISCORD_TOKEN":             "test-token",
		"DISCORD_APPLICATION_ID":    "123456789012345678",
		"DISCORD_GUILD_ID":          "123456789012345678",
		"DISCORD_ALLOWED_ROLE_IDS":  "234567890123456789,345678901234567890",
		"WOL_MAC":                   "00:11:22:33:44:55",
		"WOL_BROADCAST":             "192.168.1.255",
		"WOL_PORT":                  "9",
		"MINECRAFT_RCON_ADDRESS":    "192.168.1.50:25575",
		"MINECRAFT_RCON_PASSWORD":   "test-password",
		"HOST_SSH_ADDRESS":          "192.168.1.50:22",
		"HOST_SSH_USER":             "minecraft-bot",
		"HOST_SSH_KEY_FILE":         "/run/secrets/id_ed25519",
		"HOST_SSH_KNOWN_HOSTS_FILE": "/run/secrets/known_hosts",
		"STARTUP_TIMEOUT":           "6m",
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
}

func TestLoad(t *testing.T) {
	setValidEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WOLPort != 9 || cfg.StartupTimeout != 6*time.Minute {
		t.Errorf("unexpected WOL port or startup timeout: %d, %s", cfg.WOLPort, cfg.StartupTimeout)
	}
	if len(cfg.AllowedRoleIDs) != 2 {
		t.Errorf("got %d allowed roles, want 2", len(cfg.AllowedRoleIDs))
	}
}

func TestLoadRejectsInvalidMAC(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("WOL_MAC", "not-a-mac")

	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted an invalid Wake-on-LAN MAC address")
	}
}
