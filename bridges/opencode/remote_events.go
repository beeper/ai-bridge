package opencode

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

// -----------------------------------------------------------------------
// OpenCodeRemoteEdit — for debounced streaming edits
// -----------------------------------------------------------------------

var (
	_ bridgev2.RemoteEdit                 = (*OpenCodeRemoteEdit)(nil)
	_ bridgev2.RemoteEventWithTimestamp   = (*OpenCodeRemoteEdit)(nil)
	_ bridgev2.RemoteEventWithStreamOrder = (*OpenCodeRemoteEdit)(nil)
)

// OpenCodeRemoteEdit is a RemoteEdit for OpenCode streaming response edits.
type OpenCodeRemoteEdit struct {
	portal        networkid.PortalKey
	sender        bridgev2.EventSender
	targetMessage networkid.MessageID
	timestamp     time.Time

	preBuilt *bridgev2.ConvertedEdit
}

func (e *OpenCodeRemoteEdit) GetType() bridgev2.RemoteEventType {
	return bridgev2.RemoteEventEdit
}

func (e *OpenCodeRemoteEdit) GetPortalKey() networkid.PortalKey {
	return e.portal
}

func (e *OpenCodeRemoteEdit) AddLogContext(c zerolog.Context) zerolog.Context {
	return c.Str("opencode_edit_target", string(e.targetMessage))
}

func (e *OpenCodeRemoteEdit) GetSender() bridgev2.EventSender {
	return e.sender
}

func (e *OpenCodeRemoteEdit) GetTargetMessage() networkid.MessageID {
	return e.targetMessage
}

func (e *OpenCodeRemoteEdit) GetTimestamp() time.Time {
	if e.timestamp.IsZero() {
		return time.Now()
	}
	return e.timestamp
}

func (e *OpenCodeRemoteEdit) GetStreamOrder() int64 {
	return e.GetTimestamp().UnixMilli()
}

func (e *OpenCodeRemoteEdit) ConvertEdit(_ context.Context, _ *bridgev2.Portal, _ bridgev2.MatrixAPI, existing []*database.Message) (*bridgev2.ConvertedEdit, error) {
	if e.preBuilt != nil && len(existing) > 0 {
		for i, part := range e.preBuilt.ModifiedParts {
			if part.Part == nil && i < len(existing) {
				part.Part = existing[i]
			}
		}
	}
	return e.preBuilt, nil
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func newOpenCodeMessageID() networkid.MessageID {
	return networkid.MessageID(fmt.Sprintf("opencode:%s", uuid.NewString()))
}
