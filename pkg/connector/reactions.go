package connector

import (
	"context"
	"time"

	"go.mau.fi/util/variationselector"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// sendReaction sends a reaction emoji to a Matrix event.
// Returns the reaction event ID on success.
func (oc *AIClient) sendReaction(ctx context.Context, portal *bridgev2.Portal, targetEventID id.EventID, emoji string) id.EventID {
	if portal == nil || portal.MXID == "" || targetEventID == "" || emoji == "" {
		return ""
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return ""
	}

	targetPart, err := oc.UserLogin.Bridge.DB.Message.GetPartByMXID(ctx, targetEventID)
	if err != nil {
		oc.log.Warn().Err(err).
			Stringer("target_event", targetEventID).
			Msg("Failed to load reaction target from database")
		return ""
	}
	if targetPart == nil {
		oc.log.Warn().
			Stringer("target_event", targetEventID).
			Msg("Reaction target message not found in database")
		return ""
	}
	if targetPart.Room != portal.PortalKey {
		oc.log.Warn().
			Stringer("target_event", targetEventID).
			Stringer("target_room", targetPart.Room).
			Stringer("portal_room", portal.PortalKey).
			Msg("Reaction target message is not in the current portal")
		return ""
	}

	senderID := oc.reactionSenderID(ctx, portal)
	if senderID == "" {
		oc.log.Warn().
			Stringer("target_event", targetEventID).
			Msg("Failed to resolve reaction sender ID")
		return ""
	}

	normalizedEmoji := variationselector.Remove(emoji)
	ts := time.Now()
	dbReaction := &database.Reaction{
		Room:          portal.PortalKey,
		MessageID:     targetPart.ID,
		MessagePartID: targetPart.PartID,
		SenderID:      senderID,
		SenderMXID:    intent.GetMXID(),
		EmojiID:       networkid.EmojiID(normalizedEmoji),
		Emoji:         normalizedEmoji,
		Timestamp:     ts,
	}

	eventContent := &event.Content{
		Parsed: &event.ReactionEventContent{
			RelatesTo: event.RelatesTo{
				Type:    event.RelAnnotation,
				EventID: targetEventID,
				Key:     variationselector.Add(normalizedEmoji),
			},
		},
	}

	resp, err := intent.SendMessage(ctx, portal.MXID, event.EventReaction, eventContent, &bridgev2.MatrixSendExtra{
		Timestamp:    ts,
		ReactionMeta: dbReaction,
	})
	if err != nil {
		oc.log.Warn().Err(err).
			Stringer("target_event", targetEventID).
			Str("emoji", emoji).
			Msg("Failed to send reaction")
		return ""
	} else {
		oc.log.Debug().
			Stringer("target_event", targetEventID).
			Str("emoji", emoji).
			Stringer("reaction_event", resp.EventID).
			Msg("Sent reaction")
	}

	dbReaction.MXID = resp.EventID
	if err := oc.UserLogin.Bridge.DB.Reaction.Upsert(ctx, dbReaction); err != nil {
		oc.log.Warn().Err(err).
			Stringer("reaction_event", resp.EventID).
			Msg("Failed to store reaction in database")
	}
	return resp.EventID
}

func (oc *AIClient) reactionSenderID(ctx context.Context, portal *bridgev2.Portal) networkid.UserID {
	if portal == nil || oc == nil || oc.UserLogin == nil || oc.UserLogin.Bridge == nil {
		return ""
	}
	meta := portalMeta(portal)
	agentID := meta.AgentID
	if agentID == "" {
		agentID = meta.DefaultAgentID
	}
	modelID := oc.effectiveModel(meta)
	if agentID != "" {
		if ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, agentModelUserID(agentID, modelID)); err == nil && ghost != nil {
			return agentModelUserID(agentID, modelID)
		}
	}
	return modelUserID(modelID)
}
