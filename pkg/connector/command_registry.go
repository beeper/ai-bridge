package connector

import (
	"strings"

	"maunium.net/go/mautrix/bridgev2/commands"

	"github.com/beeper/ai-bridge/pkg/connector/commandregistry"
)

var aiCommandRegistry = commandregistry.NewRegistry()

func registerAICommand(def commandregistry.Definition) *commands.FullHandler {
	return aiCommandRegistry.Register(def)
}

// registerCommands registers all AI commands with the command processor.
func (oc *OpenAIConnector) registerCommands(proc *commands.Processor) {
	handlers := aiCommandRegistry.All()
	if len(handlers) > 0 {
		proc.AddHandlers(handlers...)
	}

	names := aiCommandRegistry.Names()
	oc.br.Log.Info().
		Str("section", HelpSectionAI.Name).
		Int("section_order", HelpSectionAI.Order).
		Strs("commands", names).
		Msg("Registered AI commands: " + strings.Join(names, ", "))
}
