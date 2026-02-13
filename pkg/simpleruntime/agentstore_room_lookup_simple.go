package connector

import (
	"context"
	"errors"

	"maunium.net/go/mautrix/bridgev2"
)

func (b *BossStoreAdapter) resolvePortalByRoomID(_ context.Context, _ string) (*bridgev2.Portal, error) {
	_ = b
	return nil, errors.New("room lookup not available in simple bridge")
}
