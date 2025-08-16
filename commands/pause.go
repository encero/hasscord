package commands

import (
	"fmt"
	"strings"

	"hasscord/bot"
	"hasscord/sensors"

	"github.com/bwmarrin/discordgo"
)

// Pause represents the pause command for door sensor notifications.
type Pause struct{}

// Name returns the command's name.
func (p *Pause) Name() string {
	return "pause"
}

// Execute runs the command.
func (p *Pause) Execute(s bot.Messager, m *discordgo.MessageCreate, args []string) {
	if len(args) == 0 {
		// Show current status
		total, paused := sensors.GetPauseStatus()
		if total == 0 {
			s.ChannelMessageSend(m.ChannelID, "ℹ️ **No doors are currently open**\n\nThere are no active door sensors to pause notifications for.")
		} else if paused == 0 {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("✅ **Door sensor notifications are ACTIVE**\n\n%d door(s) are currently open and notifications are enabled.\n\nUse `!pause on` to pause notifications for currently open doors", total))
		} else if paused == total {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("🚫 **Door sensor notifications are PAUSED**\n\n%d door(s) are currently open but notifications are paused.\n\nUse `!pause off` to resume notifications", total))
		} else {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("⚠️ **Door sensor notifications are PARTIALLY PAUSED**\n\n%d door(s) are currently open:\n• %d have notifications paused\n• %d have notifications active\n\nUse `!pause on` to pause all\nUse `!pause off` to resume all", total, paused, total-paused))
		}
		return
	}

	action := strings.ToLower(args[0])

	switch action {
	case "on", "pause", "stop":
		sensors.PauseNotifications()
		total, paused := sensors.GetPauseStatus()
		if paused > 0 {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("🚫 **Door sensor notifications PAUSED**\n\nNotifications have been paused for %d currently open door(s).\n\nThese doors will continue to be tracked but won't send notifications until you resume them or they close naturally.", paused))
		} else {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("ℹ️ **No doors to pause**\n\nThere are no currently open doors to pause notifications for. (Total open: %d)", total))
		}

	case "off", "resume", "start":
		sensors.ResumeNotifications()
		total, paused := sensors.GetPauseStatus()
		if paused == 0 && total > 0 {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("✅ **Door sensor notifications RESUMED**\n\nNotifications have been resumed for all %d currently open door(s).", total))
		} else if total == 0 {
			s.ChannelMessageSend(m.ChannelID, "ℹ️ **No doors to resume**\n\nThere are no currently open doors to resume notifications for.")
		} else {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("✅ **Door sensor notifications RESUMED**\n\nNotifications have been resumed for all %d currently open door(s).", total))
		}

	default:
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ **Invalid action: `%s`**\n\nValid actions:\n• `!pause on` - Pause notifications for currently open doors\n• `!pause off` - Resume notifications for currently open doors\n• `!pause` - Show current status", action))
	}
}
