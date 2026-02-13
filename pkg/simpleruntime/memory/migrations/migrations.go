package migrations

import (
	"embed"

	"maunium.net/go/mautrix/bridgev2/database/upgrades"
)

//go:embed *.sql
var rawUpgrades embed.FS

func init() {
	upgrades.Table.RegisterFS(rawUpgrades)
}
