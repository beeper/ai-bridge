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
		commandHandlers := make([]commands.CommandHandler, 0, len(handlers))
		for _, handler := range handlers {
			if handler == nil || handler.Func == nil {
				continue
			}
			if !oc.commandAllowed(handler.Name) {
				continue
			}
			original := handler.Func
			handler.Func = func(ce *commands.Event) {
				senderID := ""
				if ce != nil && ce.User != nil {
					senderID = ce.User.MXID.String()
				}
				if !isOwnerAllowed(&oc.Config, senderID) {
					if ce != nil {
						ce.Reply("Only configured owners can use that command.")
					}
					return
				}
				original(ce)
			}
			commandHandlers = append(commandHandlers, handler)
		}
		proc.AddHandlers(commandHandlers...)
	}

	names := aiCommandRegistry.Names()
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if oc.commandAllowed(name) {
			filtered = append(filtered, name)
		}
	}
	oc.br.Log.Info().
		Str("section", HelpSectionAI.Name).
		Int("section_order", HelpSectionAI.Order).
		Strs("commands", filtered).
		Msg("Registered AI commands: " + strings.Join(filtered, ", "))
}
