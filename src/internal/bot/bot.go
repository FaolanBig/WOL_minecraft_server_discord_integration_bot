package bot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/faolanbig/wol-minecraft-server-discord-integration-bot/src/internal/server"
)

type Bot struct {
	session        *discordgo.Session
	manager        *server.Manager
	applicationID  string
	guildID        string
	allowedRoleIDs map[string]struct{}
	startupTimeout time.Duration
}

func New(session *discordgo.Session, manager *server.Manager, applicationID, guildID string, allowedRoleIDs []string, startupTimeout time.Duration) *Bot {
	roles := make(map[string]struct{}, len(allowedRoleIDs))
	for _, roleID := range allowedRoleIDs {
		roles[roleID] = struct{}{}
	}
	return &Bot{
		session:        session,
		manager:        manager,
		applicationID:  applicationID,
		guildID:        guildID,
		allowedRoleIDs: roles,
		startupTimeout: startupTimeout,
	}
}

func (b *Bot) RegisterHandlers() {
	b.session.AddHandler(b.handleInteraction)
}

func (b *Bot) RegisterCommands(ctx context.Context) error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "start",
			Description: "Wake the Minecraft server and wait for it to become available",
		},
		{
			Name:        "status",
			Description: "Check whether the Minecraft server is available and how many players are online",
		},
	}

	if _, err := b.session.ApplicationCommandBulkOverwrite(b.applicationID, b.guildID, commands, discordgo.WithContext(ctx)); err != nil {
		return fmt.Errorf("register Discord commands: %w", err)
	}
	return nil
}

func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name == "" {
		return
	}
	if i.GuildID != b.guildID || !b.hasAllowedRole(i.Member) {
		respond(s, i.Interaction, "You do not have a configured role that can use this bot.")
		return
	}

	switch i.ApplicationCommandData().Name {
	case "start":
		b.start(s, i.Interaction)
	case "status":
		b.status(s, i.Interaction)
	}
}

func (b *Bot) hasAllowedRole(member *discordgo.Member) bool {
	if member == nil {
		return false
	}
	for _, roleID := range member.Roles {
		if _, allowed := b.allowedRoleIDs[roleID]; allowed {
			return true
		}
	}
	return false
}

func (b *Bot) start(s *discordgo.Session, i *discordgo.Interaction) {
	err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})
	if err != nil {
		slog.Error("failed to acknowledge /start interaction", "error", err)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), b.startupTimeout)
		defer cancel()

		alreadyOnline, err := b.manager.Start(ctx)
		message := "Minecraft server is online and ready."
		if err != nil {
			message = "Could not confirm that the Minecraft server became ready. Please check the bot and server logs."
			slog.Error("Minecraft server start request failed", "error", err)
		} else if alreadyOnline {
			message = "Minecraft server is already online."
		}
		if _, err := s.InteractionResponseEdit(i, &discordgo.WebhookEdit{Content: &message}); err != nil {
			slog.Error("failed to update /start response", "error", err)
		}
	}()
}

func (b *Bot) status(s *discordgo.Session, i *discordgo.Interaction) {
	if err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	}); err != nil {
		slog.Error("failed to acknowledge /status interaction", "error", err)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()

		players, err := b.manager.Status(ctx)
		message := fmt.Sprintf("Minecraft server is online; %d player(s) online.", players)
		if err != nil {
			message = "Minecraft server is not responding via RCON. It may be powered off or still starting."
			slog.Warn("Minecraft status request failed", "error", err)
		}
		if _, err := s.InteractionResponseEdit(i, &discordgo.WebhookEdit{Content: &message}); err != nil {
			slog.Error("failed to update /status response", "error", err)
		}
	}()
}

func respond(s *discordgo.Session, i *discordgo.Interaction, message string) {
	if err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		slog.Error("failed to respond to Discord interaction", "error", err)
	}
}
