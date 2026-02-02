package connector

import (
	"context"
	"encoding/json"
	"fmt"

	"maunium.net/go/mautrix/id"
)

// executeMessageRead handles the read action - sends a read receipt.
func executeMessageRead(ctx context.Context, args map[string]any, btc *BridgeToolContext) (string, error) {
	// Get target message ID (optional - defaults to triggering message)
	var targetEventID id.EventID
	if msgID, ok := args["message_id"].(string); ok && msgID != "" {
		targetEventID = id.EventID(msgID)
	} else if btc.SourceEventID != "" {
		targetEventID = btc.SourceEventID
	}

	if targetEventID == "" {
		return "", fmt.Errorf("action=read requires 'message_id' parameter (no triggering message available)")
	}

	err := sendMatrixReadReceipt(ctx, btc, targetEventID)
	if err != nil {
		return "", fmt.Errorf("failed to send read receipt: %w", err)
	}

	return fmt.Sprintf(`{"action":"read","message_id":%q,"status":"sent"}`, targetEventID), nil
}

// executeMessageChannelInfo handles the channel-info action - gets room information.
func executeMessageChannelInfo(ctx context.Context, _ map[string]any, btc *BridgeToolContext) (string, error) {
	info, err := getMatrixRoomInfo(ctx, btc)
	if err != nil {
		return "", fmt.Errorf("failed to get room info: %w", err)
	}

	if info == nil {
		return "", fmt.Errorf("room info not available")
	}

	result, _ := json.Marshal(map[string]any{
		"action":       "channel-info",
		"room_id":      info.RoomID,
		"name":         info.Name,
		"topic":        info.Topic,
		"member_count": info.MemberCount,
	})
	return string(result), nil
}

// executeMessageMemberInfo handles the member-info action - gets user profile.
func executeMessageMemberInfo(ctx context.Context, args map[string]any, btc *BridgeToolContext) (string, error) {
	userIDStr, ok := args["user_id"].(string)
	if !ok || userIDStr == "" {
		return "", fmt.Errorf("action=member-info requires 'user_id' parameter")
	}

	userID := id.UserID(userIDStr)
	profile, err := getMatrixUserProfile(ctx, btc, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	if profile == nil {
		return "", fmt.Errorf("user profile not available")
	}

	result, _ := json.Marshal(map[string]any{
		"action":       "member-info",
		"user_id":      profile.UserID,
		"display_name": profile.DisplayName,
		"avatar_url":   profile.AvatarURL,
	})
	return string(result), nil
}

// executeMessageReactions handles the reactions action - lists reactions on a message.
func executeMessageReactions(ctx context.Context, args map[string]any, btc *BridgeToolContext) (string, error) {
	// Get target message ID (required for listing reactions)
	msgID, ok := args["message_id"].(string)
	if !ok || msgID == "" {
		return "", fmt.Errorf("action=reactions requires 'message_id' parameter")
	}
	targetEventID := id.EventID(msgID)

	reactions, err := listMatrixReactions(ctx, btc, targetEventID)
	if err != nil {
		return "", fmt.Errorf("failed to list reactions: %w", err)
	}

	result, _ := json.Marshal(map[string]any{
		"action":     "reactions",
		"message_id": msgID,
		"reactions":  reactions,
		"count":      len(reactions),
	})
	return string(result), nil
}

// executeMessageReactRemove handles reaction removal - removes the bot's reactions.
func executeMessageReactRemove(ctx context.Context, args map[string]any, btc *BridgeToolContext) (string, error) {
	// Get target message ID
	var targetEventID id.EventID
	if msgID, ok := args["message_id"].(string); ok && msgID != "" {
		targetEventID = id.EventID(msgID)
	} else if btc.SourceEventID != "" {
		targetEventID = btc.SourceEventID
	}

	if targetEventID == "" {
		return "", fmt.Errorf("action=react with remove requires 'message_id' parameter")
	}

	// Get emoji to remove (empty means all)
	emoji, _ := args["emoji"].(string)

	removed, err := removeMatrixReactions(ctx, btc, targetEventID, emoji)
	if err != nil {
		return "", fmt.Errorf("failed to remove reactions: %w", err)
	}

	return fmt.Sprintf(`{"action":"react","emoji":%q,"message_id":%q,"removed":%d,"status":"removed"}`, emoji, targetEventID, removed), nil
}
