package stream

import (
	"context"

	"github.com/beeper/ai-bridge/modules/contracts/matrixevents"
	"github.com/beeper/ai-bridge/pkg/matrixtransport"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Emitter sends AI SDK compatible streaming events over Matrix.
//
// This is transport-agnostic and can be used by the bridge adapter, bot, and
// future modules that need token/tool streaming semantics.
type Emitter struct {
	T matrixtransport.Transport
}

type PartEnvelope struct {
	TurnID         string
	Seq            int
	TargetEventID  id.EventID
	AgentID        string
	Part           map[string]any
	TransactionID  string
	EphemeralType  event.Type
	OverrideRoomID *id.RoomID
}

func (e *Emitter) Emit(ctx context.Context, roomID id.RoomID, env PartEnvelope) error {
	if e == nil || e.T == nil {
		return nil
	}
	if env.EphemeralType.Type == "" {
		env.EphemeralType = matrixevents.StreamEventMessageType
	}
	contentRaw, err := matrixevents.BuildStreamEventEnvelope(env.TurnID, env.Seq, env.Part, matrixevents.StreamEventOpts{
		TargetEventID: env.TargetEventID.String(),
		AgentID:       env.AgentID,
	})
	if err != nil {
		return err
	}
	if env.TransactionID == "" {
		env.TransactionID = matrixevents.BuildStreamEventTxnID(env.TurnID, env.Seq)
	}
	finalRoomID := roomID
	if env.OverrideRoomID != nil && *env.OverrideRoomID != "" {
		finalRoomID = *env.OverrideRoomID
	}
	return e.T.SendEphemeral(ctx, finalRoomID, env.EphemeralType, &event.Content{Raw: contentRaw}, env.TransactionID)
}
