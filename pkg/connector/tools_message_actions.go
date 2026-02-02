package connector

import (
	"context"
	"fmt"
	"strings"

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

	return jsonActionResult("read", map[string]any{
		"message_id": targetEventID,
		"status":     "sent",
	})
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

	return jsonActionResult("channel-info", map[string]any{
		"room_id":      info.RoomID,
		"name":         info.Name,
		"topic":        info.Topic,
		"member_count": info.MemberCount,
	})
}

// executeMessageChannelEdit handles channel-edit by mapping to room title/topic updates.
func executeMessageChannelEdit(ctx context.Context, args map[string]any, btc *BridgeToolContext) (string, error) {
	var title string
	if raw, ok := args["name"]; ok {
		if s, ok := raw.(string); ok {
			title = strings.TrimSpace(s)
		} else {
			return "", fmt.Errorf("action=channel-edit requires 'name' to be a string")
		}
	}
	if title == "" {
		if raw, ok := args["title"]; ok {
			if s, ok := raw.(string); ok {
				title = strings.TrimSpace(s)
			} else {
				return "", fmt.Errorf("action=channel-edit requires 'title' to be a string")
			}
		}
	}

	descProvided := false
	description := ""
	if raw, ok := args["topic"]; ok {
		descProvided = true
		if s, ok := raw.(string); ok {
			description = strings.TrimSpace(s)
		} else {
			return "", fmt.Errorf("action=channel-edit requires 'topic' to be a string")
		}
	} else if raw, ok := args["description"]; ok {
		descProvided = true
		if s, ok := raw.(string); ok {
			description = strings.TrimSpace(s)
		} else {
			return "", fmt.Errorf("action=channel-edit requires 'description' to be a string")
		}
	}

	if title == "" && !descProvided {
		return "", fmt.Errorf("action=channel-edit requires 'name' or 'topic'")
	}

	if btc == nil {
		btc = GetBridgeToolContext(ctx)
	}
	if btc == nil {
		return "", fmt.Errorf("bridge context not available")
	}
	if btc.Portal == nil {
		return "", fmt.Errorf("portal not available")
	}

	updates := make([]string, 0, 2)
	if title != "" {
		if err := btc.Client.setRoomName(ctx, btc.Portal, title); err != nil {
			return "", fmt.Errorf("failed to set room title: %w", err)
		}
		updates = append(updates, fmt.Sprintf("title=%s", title))
	}
	if descProvided {
		if err := btc.Client.setRoomTopic(ctx, btc.Portal, description); err != nil {
			return "", fmt.Errorf("failed to set room description: %w", err)
		}
		if description == "" {
			updates = append(updates, "description=cleared")
		} else {
			updates = append(updates, fmt.Sprintf("description=%s", description))
		}
	}

	result := map[string]any{
		"status":  "updated",
		"updates": updates,
	}
	if title != "" {
		result["title"] = title
	}
	if descProvided {
		result["description"] = description
	}

	return jsonActionResult("channel-edit", result)
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

	return jsonActionResult("member-info", map[string]any{
		"user_id":      profile.UserID,
		"display_name": profile.DisplayName,
		"avatar_url":   profile.AvatarURL,
	})
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

	return jsonActionResult("reactions", map[string]any{
		"message_id": msgID,
		"reactions":  reactions,
		"count":      len(reactions),
	})
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

	return jsonActionResult("react", map[string]any{
		"emoji":      emoji,
		"message_id": targetEventID,
		"removed":    removed,
		"status":     "removed",
	})
}
