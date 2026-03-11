package codex

import (
	"strings"
	"time"

	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"

	"github.com/beeper/agentremote/pkg/bridgeadapter"
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
	bridgeadapter.BaseMessageMetadata
	ExcludeFromHistory bool   `json:"exclude_from_history,omitempty"`
	CompletionID       string `json:"completion_id,omitempty"`
	Model              string `json:"model,omitempty"`
	HasToolCalls       bool   `json:"has_tool_calls,omitempty"`
	Transcript         string `json:"transcript,omitempty"`
	FirstTokenAtMs     int64  `json:"first_token_at_ms,omitempty"`
	ThinkingTokenCount int    `json:"thinking_token_count,omitempty"`
}

type ToolCallMetadata = bridgeadapter.ToolCallMetadata

type GeneratedFileRef = bridgeadapter.GeneratedFileRef

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
	if src.ExcludeFromHistory {
		mm.ExcludeFromHistory = true
	}
	if src.CompletionID != "" {
		mm.CompletionID = src.CompletionID
	}
	if src.Model != "" {
		mm.Model = src.Model
	}
	if src.HasToolCalls {
		mm.HasToolCalls = true
	}
	if src.Transcript != "" {
		mm.Transcript = src.Transcript
	}
	if src.FirstTokenAtMs != 0 {
		mm.FirstTokenAtMs = src.FirstTokenAtMs
	}
	if src.ThinkingTokenCount != 0 {
		mm.ThinkingTokenCount = src.ThinkingTokenCount
	}
}

func loginMetadata(login *bridgev2.UserLogin) *UserLoginMetadata {
	return bridgeadapter.EnsureLoginMetadata[UserLoginMetadata](login)
}

func portalMeta(portal *bridgev2.Portal) *PortalMetadata {
	return bridgeadapter.EnsurePortalMetadata[PortalMetadata](portal)
}

func normalizedCodexAuthSource(meta *UserLoginMetadata) string {
	if meta == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(meta.CodexAuthSource))
}

func isHostAuthLogin(meta *UserLoginMetadata) bool {
	source := normalizedCodexAuthSource(meta)
	if source == CodexAuthSourceHost {
		return true
	}
	// Backward-compatible fallback for older host-auth auto-provisioned logins.
	if source == "" && meta != nil && !meta.CodexHomeManaged && strings.TrimSpace(meta.CodexHome) == "" {
		return true
	}
	return false
}

func isManagedAuthLogin(meta *UserLoginMetadata) bool {
	source := normalizedCodexAuthSource(meta)
	if source == CodexAuthSourceManaged {
		return true
	}
	if source == CodexAuthSourceHost {
		return false
	}
	return meta != nil && meta.CodexHomeManaged
}

func NewTurnID() string {
	return "turn_" + strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000"), ".", "")
}
