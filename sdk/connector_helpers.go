package sdk

import (
	"maunium.net/go/mautrix/bridgev2/database"

	"github.com/beeper/agentremote"
)

// BuildStandardMetaTypes returns the common bridge metadata registrations.
func BuildStandardMetaTypes(
	newPortal func() any,
	newMessage func() any,
	newLogin func() any,
	newGhost func() any,
) database.MetaTypes {
	return agentremote.BuildMetaTypes(newPortal, newMessage, newLogin, newGhost)
}

// ApplyDefaultCommandPrefix sets the command prefix when it is empty.
func ApplyDefaultCommandPrefix(prefix *string, value string) {
	if prefix != nil && *prefix == "" {
		*prefix = value
	}
}

// ApplyBoolDefault initializes a nil bool pointer to the provided value.
func ApplyBoolDefault(target **bool, value bool) {
	if target == nil || *target != nil {
		return
	}
	v := value
	*target = &v
}
