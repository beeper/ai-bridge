package codex

import (
	"strings"

	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"

	"github.com/beeper/agentremote"
)

type UserLoginMetadata struct {
	Provider          string `json:"provider,omitempty"`
	CodexHome         string `json:"codex_home,omitempty"`
	CodexHomeManaged  bool   `json:"codex_home_managed,omitempty"`
	CodexAuthSource   string `json:"codex_auth_source,omitempty"`
	CodexCommand      string `json:"codex_command,omitempty"`
	CodexAuthMode     string `json:"codex_auth_mode,omitempty"`
	CodexAccountEmail string `json:"codex_account_email,omitempty"`
	ChatsSynced       bool   `json:"chats_synced,omitempty"`
}

const (
	CodexAuthSourceManaged = "managed"
	CodexAuthSourceHost    = "host"
)

type PortalMetadata struct {
	Title            string `json:"title,omitempty"`
	Slug             string `json:"slug,omitempty"`
	IsCodexRoom      bool   `json:"is_codex_room,omitempty"`
	CodexThreadID    string `json:"codex_thread_id,omitempty"`
	CodexCwd         string `json:"codex_cwd,omitempty"`
	ElevatedLevel    string `json:"elevated_level,omitempty"`
	AwaitingCwdSetup bool   `json:"awaiting_cwd_setup,omitempty"`
}

type MessageMetadata struct {
	agentremote.BaseMessageMetadata
	agentremote.AssistantMessageMetadata
}

type ToolCallMetadata = agentremote.ToolCallMetadata

type GhostMetadata struct {
	LastSync jsontime.Unix `json:"last_sync,omitempty"`
}

var _ database.MetaMerger = (*MessageMetadata)(nil)

func (mm *MessageMetadata) CopyFrom(other any) {
	src, ok := other.(*MessageMetadata)
	if !ok || src == nil {
		return
	}
	mm.CopyFromBase(&src.BaseMessageMetadata)
	mm.CopyFromAssistant(&src.AssistantMessageMetadata)
}

func loginMetadata(login *bridgev2.UserLogin) *UserLoginMetadata {
	return agentremote.EnsureLoginMetadata[UserLoginMetadata](login)
}

func portalMeta(portal *bridgev2.Portal) *PortalMetadata {
	return agentremote.EnsurePortalMetadata[PortalMetadata](portal)
}

func normalizedCodexAuthSource(meta *UserLoginMetadata) string {
	if meta == nil {
		return ""
	}
	if source := strings.ToLower(strings.TrimSpace(meta.CodexAuthSource)); source != "" {
		return source
	}
	if meta.CodexHomeManaged {
		return CodexAuthSourceManaged
	}
	return ""
}

func isHostAuthLogin(meta *UserLoginMetadata) bool {
	return normalizedCodexAuthSource(meta) == CodexAuthSourceHost
}

func isManagedAuthLogin(meta *UserLoginMetadata) bool {
	return normalizedCodexAuthSource(meta) == CodexAuthSourceManaged
}

