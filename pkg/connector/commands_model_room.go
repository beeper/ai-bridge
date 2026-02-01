package connector

import (
	"maunium.net/go/mautrix/bridgev2/commands"
)

// CommandModelRoom handles the !ai model-room command.
// This creates a raw model chat without an agent wrapper for advanced users.
var CommandModelRoom = &commands.FullHandler{
	Func: fnModelRoom,
	Name: "model-room",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Create a raw model chat (no agent)",
		Args:        "<model>",
	},
	RequiresLogin: true,
}

func fnModelRoom(ce *commands.Event) {
	client := getAIClient(ce)
	if client == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	if len(ce.Args) == 0 {
		ce.Reply("Usage: !ai model-room <model>\n\nExample: !ai model-room gpt-4o")
		return
	}

	modelID := ce.Args[0]

	// Validate the model exists
	valid, err := client.validateModel(ce.Ctx, modelID)
	if err != nil || !valid {
		ce.Reply("Invalid model: %s", modelID)
		return
	}

	// Create a raw model room (no agent)
	go func() {
		chatResp, err := client.createNewChat(ce.Ctx, modelID)
		if err != nil {
			client.log.Err(err).Str("model", modelID).Msg("Failed to create model room")
			return
		}

		if chatResp != nil && chatResp.Portal != nil && chatResp.Portal.MXID != "" {
			client.sendSystemNotice(ce.Ctx, ce.Portal,
				"Created raw model room: "+string(chatResp.Portal.MXID))
		}
	}()

	ce.Reply("Creating raw model room with %s...", modelID)
}
