package connector

import (
	"maunium.net/go/mautrix/bridgev2/commands"

	"github.com/beeper/ai-bridge/pkg/connector/commandregistry"
)

// CommandPlayground handles the !ai playground command.
// This creates a playground room with minimal tools and no agent personality.
var CommandPlayground = registerAICommand(commandregistry.Definition{
	Name:          "playground",
	Aliases:       []string{"sandbox"},
	Description:   "Create a model playground chat (minimal tools, no personality)",
	Args:          "<model>",
	Section:       HelpSectionAI,
	RequiresLogin: true,
	Handler:       fnPlayground,
})

func fnPlayground(ce *commands.Event) {
	client, ok := requireClient(ce)
	if !ok {
		return
	}

	if len(ce.Args) == 0 {
		ce.Reply("Usage: !ai playground <model>\n\nExample: !ai playground claude-sonnet-4.5\n\nThis creates a raw model sandbox with minimal tools and no agent personality.")
		return
	}

	modelArg := ce.Args[0]

	// Resolve the model (handles aliases, prefixes, etc.)
	modelID, valid, err := client.resolveModelID(ce.Ctx, modelArg)
	if err != nil || !valid || modelID == "" {
		ce.Reply("That model isn't available: %s", modelArg)
		return
	}

	// Create a raw model sandbox with the specified model.
	go func() {
		client.createAndOpenModelChat(ce.Ctx, ce.Portal, modelID)
	}()

	ce.Reply("Creating playground room with %s...", modelID)
}
