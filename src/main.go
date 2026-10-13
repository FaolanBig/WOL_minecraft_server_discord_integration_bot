package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/faolanbig/wol-minecraft-server-discord-integration-bot/src/internal/bot"
	"github.com/faolanbig/wol-minecraft-server-discord-integration-bot/src/internal/config"
	"github.com/faolanbig/wol-minecraft-server-discord-integration-bot/src/internal/host"
	"github.com/faolanbig/wol-minecraft-server-discord-integration-bot/src/internal/minecraft"
	"github.com/faolanbig/wol-minecraft-server-discord-integration-bot/src/internal/server"
	"github.com/faolanbig/wol-minecraft-server-discord-integration-bot/src/internal/wol"
)

func main() {
	if err := run(); err != nil {
		slog.Error("bot stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	mc := minecraft.NewRCON(cfg.RCONAddress, cfg.RCONPassword)
	hostControl, err := host.NewSSH(cfg.HostSSHAddress, cfg.HostSSHUser, cfg.HostSSHKeyFile, cfg.KnownHostsFile)
	if err != nil {
		return err
	}
	manager := server.NewManager(mc, wol.New(cfg.WOLMAC, cfg.WOLBroadcast, cfg.WOLPort), hostControl)

	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return err
	}

	client := bot.New(session, manager, cfg.ApplicationID, cfg.GuildID, cfg.AllowedRoleIDs, cfg.StartupTimeout)
	client.RegisterHandlers()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := session.Open(); err != nil {
		return err
	}
	defer session.Close()

	if err := client.RegisterCommands(ctx); err != nil {
		return err
	}
	slog.Info("Discord bot connected", "guild_id", cfg.GuildID)

	go manager.Monitor(ctx)
	<-ctx.Done()
	slog.Info("shutdown signal received")
	return nil
}
