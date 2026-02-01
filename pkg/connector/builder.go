package connector

import (
	"context"
	"fmt"

	"github.com/beeper/ai-bridge/pkg/agents"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

// Builder room constants
const (
	BuilderRoomSlug = "builder"
	BuilderRoomName = "Manage AI Chats"
)

// ensureBuilderRoom creates or retrieves the "Manage AI Chats" room.
// This special room is where users interact with the Boss agent to manage their agents and rooms.
func (oc *AIClient) ensureBuilderRoom(ctx context.Context) error {
	meta := loginMetadata(oc.UserLogin)

	// Check if we already have a Builder room
	if meta.BuilderRoomID != "" {
		// Verify it still exists
		portal, err := oc.UserLogin.Bridge.GetPortalByKey(ctx, networkid.PortalKey{
			ID:       meta.BuilderRoomID,
			Receiver: oc.UserLogin.ID,
		})
		if err == nil && portal != nil && portal.MXID != "" {
			oc.log.Debug().Str("room_id", string(meta.BuilderRoomID)).Msg("Manage AI Chats room already exists")
			return nil
		}
		// Room doesn't exist anymore, clear the reference
		meta.BuilderRoomID = ""
	}

	oc.log.Info().Msg("Creating Manage AI Chats room")

	// Create the Builder room with Boss agent as the ghost
	portal, chatInfo, err := oc.createBuilderRoom(ctx)
	if err != nil {
		return fmt.Errorf("failed to create builder room: %w", err)
	}

	// Create Matrix room
	if err := portal.CreateMatrixRoom(ctx, oc.UserLogin, chatInfo); err != nil {
		return fmt.Errorf("failed to create matrix room for builder: %w", err)
	}

	// Store the Builder room ID
	meta.BuilderRoomID = portal.PortalKey.ID
	if err := oc.UserLogin.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save BuilderRoomID")
	}

	oc.log.Info().
		Str("portal_id", string(portal.PortalKey.ID)).
		Str("mxid", string(portal.MXID)).
		Msg("Manage AI Chats room created")

	return nil
}

// createBuilderRoom creates the "Manage AI Chats" room portal and chat info.
func (oc *AIClient) createBuilderRoom(ctx context.Context) (*bridgev2.Portal, *bridgev2.ChatInfo, error) {
	bossAgent := agents.GetBossAgent()

	// Use a standard chat initialization with the management room title
	opts := PortalInitOpts{
		Title: BuilderRoomName,
	}

	portal, chatInfo, err := oc.initPortalForChat(ctx, opts)
	if err != nil {
		return nil, nil, err
	}

	// Set up the portal metadata for the Boss agent
	pm := portalMeta(portal)
	pm.Slug = BuilderRoomSlug // Override slug to "builder"
	pm.AgentID = bossAgent.ID
	pm.DefaultAgentID = bossAgent.ID
	pm.SystemPrompt = agents.BossSystemPrompt
	pm.Model = bossAgent.Model.Primary // Explicit model - always use Boss agent's model
	pm.IsBuilderRoom = true            // Mark as protected from overrides

	// Re-save portal with updated metadata
	if err := portal.Save(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to save portal with agent config: %w", err)
	}

	return portal, chatInfo, nil
}

// isBuilderRoom checks if a portal is the Builder room.
func (oc *AIClient) isBuilderRoom(portal *bridgev2.Portal) bool {
	meta := loginMetadata(oc.UserLogin)
	return meta.BuilderRoomID != "" && portal.PortalKey.ID == meta.BuilderRoomID
}
