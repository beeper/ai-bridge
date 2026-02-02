package connector

import (
	"context"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2/matrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// getMatrixClient returns the mautrix.Client from the bridge framework.
// This provides access to Matrix API methods like GetRelations, GetProfile, etc.
func getMatrixClient(btc *BridgeToolContext) *mautrix.Client {
	matrixConn, ok := btc.Client.UserLogin.Bridge.Matrix.(*matrix.Connector)
	if !ok {
		return nil
	}
	return matrixConn.AS.BotClient()
}

// getMatrixConnector returns the matrix.Connector for state queries.
func getMatrixConnector(btc *BridgeToolContext) *matrix.Connector {
	matrixConn, ok := btc.Client.UserLogin.Bridge.Matrix.(*matrix.Connector)
	if !ok {
		return nil
	}
	return matrixConn
}

// MatrixReactionSummary represents a summary of reactions on a message.
type MatrixReactionSummary struct {
	Key   string   `json:"key"`   // The emoji
	Count int      `json:"count"` // Number of reactions with this emoji
	Users []string `json:"users"` // User IDs who reacted
}

// listMatrixReactions lists all reactions on a message using the relations API.
func listMatrixReactions(ctx context.Context, btc *BridgeToolContext, eventID id.EventID) ([]MatrixReactionSummary, error) {
	client := getMatrixClient(btc)
	if client == nil {
		return nil, nil
	}

	// Query relations API for reactions (m.annotation)
	resp, err := client.GetRelations(ctx, btc.Portal.MXID, eventID, &mautrix.ReqGetRelations{
		RelationType: event.RelAnnotation,
		EventType:    event.EventReaction,
		Limit:        100,
	})
	if err != nil {
		return nil, err
	}

	// Aggregate reactions by emoji
	summaries := make(map[string]*MatrixReactionSummary)
	for _, evt := range resp.Chunk {
		content, ok := evt.Content.Parsed.(*event.ReactionEventContent)
		if !ok {
			// Try to parse from raw
			if relatesTo, ok := evt.Content.Raw["m.relates_to"].(map[string]any); ok {
				if key, ok := relatesTo["key"].(string); ok && key != "" {
					sender := evt.Sender.String()
					if summaries[key] == nil {
						summaries[key] = &MatrixReactionSummary{Key: key, Count: 0, Users: []string{}}
					}
					summaries[key].Count++
					summaries[key].Users = append(summaries[key].Users, sender)
				}
			}
			continue
		}
		if content.RelatesTo.Key == "" {
			continue
		}
		key := content.RelatesTo.Key
		sender := evt.Sender.String()
		if summaries[key] == nil {
			summaries[key] = &MatrixReactionSummary{Key: key, Count: 0, Users: []string{}}
		}
		summaries[key].Count++
		summaries[key].Users = append(summaries[key].Users, sender)
	}

	// Convert to slice
	result := make([]MatrixReactionSummary, 0, len(summaries))
	for _, summary := range summaries {
		result = append(result, *summary)
	}
	return result, nil
}

// removeMatrixReactions removes the bot's reactions from a message.
// If emoji is specified, only removes that specific reaction.
// If emoji is empty, removes all of the bot's reactions.
func removeMatrixReactions(ctx context.Context, btc *BridgeToolContext, eventID id.EventID, emoji string) (int, error) {
	client := getMatrixClient(btc)
	if client == nil {
		return 0, nil
	}

	// Query relations API for reactions
	resp, err := client.GetRelations(ctx, btc.Portal.MXID, eventID, &mautrix.ReqGetRelations{
		RelationType: event.RelAnnotation,
		EventType:    event.EventReaction,
		Limit:        200,
	})
	if err != nil {
		return 0, err
	}

	// Get bot's user ID
	botMXID := client.UserID

	// Find reaction events from the bot that match
	var toRemove []id.EventID
	for _, evt := range resp.Chunk {
		if evt.Sender != botMXID {
			continue
		}

		// Check emoji if specified
		if emoji != "" {
			var key string
			if content, ok := evt.Content.Parsed.(*event.ReactionEventContent); ok {
				key = content.RelatesTo.Key
			} else if relatesTo, ok := evt.Content.Raw["m.relates_to"].(map[string]any); ok {
				key, _ = relatesTo["key"].(string)
			}
			if key != emoji {
				continue
			}
		}

		toRemove = append(toRemove, evt.ID)
	}

	// Redact each reaction event
	removed := 0
	for _, evtID := range toRemove {
		_, err := client.RedactEvent(ctx, btc.Portal.MXID, evtID)
		if err == nil {
			removed++
		}
	}

	return removed, nil
}

// sendMatrixReadReceipt sends a read receipt for a message.
func sendMatrixReadReceipt(ctx context.Context, btc *BridgeToolContext, eventID id.EventID) error {
	client := getMatrixClient(btc)
	if client == nil {
		return nil
	}
	return client.MarkRead(ctx, btc.Portal.MXID, eventID)
}

// MatrixUserProfile represents a user's profile information.
type MatrixUserProfile struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

// getMatrixUserProfile gets a user's profile information.
func getMatrixUserProfile(ctx context.Context, btc *BridgeToolContext, userID id.UserID) (*MatrixUserProfile, error) {
	client := getMatrixClient(btc)
	if client == nil {
		return nil, nil
	}

	profile, err := client.GetProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &MatrixUserProfile{
		UserID:      userID.String(),
		DisplayName: profile.DisplayName,
		AvatarURL:   profile.AvatarURL.String(),
	}, nil
}

// MatrixRoomInfo represents room information.
type MatrixRoomInfo struct {
	RoomID      string `json:"room_id"`
	Name        string `json:"name,omitempty"`
	Topic       string `json:"topic,omitempty"`
	MemberCount int    `json:"member_count,omitempty"`
}

// getMatrixRoomInfo gets information about a room.
func getMatrixRoomInfo(ctx context.Context, btc *BridgeToolContext) (*MatrixRoomInfo, error) {
	matrixConn := getMatrixConnector(btc)
	if matrixConn == nil {
		return nil, nil
	}

	info := &MatrixRoomInfo{
		RoomID: btc.Portal.MXID.String(),
	}

	// Get room name
	nameEvt, err := matrixConn.GetStateEvent(ctx, btc.Portal.MXID, event.StateRoomName, "")
	if err == nil && nameEvt != nil {
		if content, ok := nameEvt.Content.Parsed.(*event.RoomNameEventContent); ok {
			info.Name = content.Name
		}
	}

	// Get room topic
	topicEvt, err := matrixConn.GetStateEvent(ctx, btc.Portal.MXID, event.StateTopic, "")
	if err == nil && topicEvt != nil {
		if content, ok := topicEvt.Content.Parsed.(*event.TopicEventContent); ok {
			info.Topic = content.Topic
		}
	}

	// Get member count using the client
	client := getMatrixClient(btc)
	if client != nil {
		members, err := client.JoinedMembers(ctx, btc.Portal.MXID)
		if err == nil {
			info.MemberCount = len(members.Joined)
		}
	}

	return info, nil
}
