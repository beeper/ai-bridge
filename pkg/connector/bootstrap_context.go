package connector

import (
	"context"
	"path"
	"strings"
	"time"

	"github.com/beeper/ai-bridge/pkg/agents"
	"github.com/beeper/ai-bridge/pkg/textfs"
)

func (oc *AIClient) buildBootstrapContextFiles(ctx context.Context, agentID string, meta *PortalMetadata) []agents.EmbeddedContextFile {
	if oc == nil || oc.UserLogin == nil || oc.UserLogin.Bridge == nil || oc.UserLogin.Bridge.DB == nil {
		return nil
	}
	if strings.TrimSpace(agentID) == "" {
		agentID = "default"
	}
	store := textfs.NewStore(
		oc.UserLogin.Bridge.DB.Database,
		string(oc.UserLogin.Bridge.DB.BridgeID),
		string(oc.UserLogin.ID),
		agentID,
	)

	skipBootstrap := false
	if oc.connector != nil && oc.connector.Config.Agents != nil && oc.connector.Config.Agents.Defaults != nil {
		skipBootstrap = oc.connector.Config.Agents.Defaults.SkipBootstrap
	}
	if !skipBootstrap {
		if _, err := agents.EnsureBootstrapFiles(ctx, store); err != nil {
			oc.log.Warn().Err(err).Msg("failed to ensure workspace bootstrap files")
		}
	}

	files, err := agents.LoadBootstrapFiles(ctx, store)
	if err != nil {
		oc.log.Warn().Err(err).Msg("failed to load workspace bootstrap files")
		return nil
	}
	if meta != nil && strings.TrimSpace(meta.SubagentParentRoomID) != "" {
		files = agents.FilterBootstrapFilesForSession(files, true)
	}

	maxChars := agents.DefaultBootstrapMaxChars
	if oc.connector != nil && oc.connector.Config.Agents != nil && oc.connector.Config.Agents.Defaults != nil {
		if oc.connector.Config.Agents.Defaults.BootstrapMaxChars > 0 {
			maxChars = oc.connector.Config.Agents.Defaults.BootstrapMaxChars
		}
	}

	warn := func(message string) {
		oc.log.Warn().Msg(message)
	}
	contextFiles := agents.BuildBootstrapContextFiles(files, maxChars, warn)
	return oc.applySoulEvilToContextFiles(ctx, store, contextFiles, maxChars)
}

func (oc *AIClient) applySoulEvilToContextFiles(
	ctx context.Context,
	store *textfs.Store,
	files []agents.EmbeddedContextFile,
	maxChars int,
) []agents.EmbeddedContextFile {
	if oc == nil || oc.connector == nil || oc.connector.Config.Agents == nil || oc.connector.Config.Agents.Defaults == nil {
		return files
	}
	config := oc.connector.Config.Agents.Defaults.SoulEvil
	if config == nil {
		return files
	}
	userTimezone, _ := oc.resolveUserTimezone()
	decision := agents.DecideSoulEvil(agents.SoulEvilCheckParams{
		Config:       config,
		UserTimezone: userTimezone,
		Now:          time.Now(),
	})
	if !decision.UseEvil {
		return files
	}

	entry, found, err := store.Read(ctx, decision.FileName)
	if err != nil || !found {
		oc.log.Warn().
			Str("reason", decision.Reason).
			Str("file", decision.FileName).
			Msg("SOUL_EVIL active but file missing")
		return files
	}
	if strings.TrimSpace(entry.Content) == "" {
		oc.log.Warn().
			Str("reason", decision.Reason).
			Str("file", decision.FileName).
			Msg("SOUL_EVIL active but file empty")
		return files
	}

	soulIndex := findSoulFileIndex(files)
	if soulIndex == -1 {
		oc.log.Warn().
			Str("reason", decision.Reason).
			Msg("SOUL_EVIL active but SOUL.md not in bootstrap files")
		return files
	}

	trimmed := agents.TrimBootstrapContent(entry.Content, agents.DefaultSoulFilename, maxChars)
	if strings.TrimSpace(trimmed.Content) == "" {
		oc.log.Warn().
			Str("reason", decision.Reason).
			Msg("SOUL_EVIL active but trimmed content empty")
		return files
	}

	files[soulIndex].Content = trimmed.Content
	oc.log.Debug().
		Str("reason", decision.Reason).
		Str("file", decision.FileName).
		Msg("SOUL_EVIL active using file")
	return files
}

func findSoulFileIndex(files []agents.EmbeddedContextFile) int {
	for idx, file := range files {
		normalized := strings.ReplaceAll(strings.TrimSpace(file.Path), "\\", "/")
		base := path.Base(normalized)
		if strings.EqualFold(base, agents.DefaultSoulFilename) {
			return idx
		}
	}
	return -1
}
