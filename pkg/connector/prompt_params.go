package connector

import (
	"os"
	"path/filepath"
	"strings"
)

func resolvePromptWorkspaceDir() string {
	return "/"
}

func resolvePromptRepoRoot(workspaceDir string) string {
	return ""
}

//lint:ignore U1000 Reserved for future prompt context wiring.
func findGitRoot(startDir string) string {
	current := startDir
	for i := 0; i < 12; i++ {
		gitPath := filepath.Join(current, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			if info.IsDir() || info.Mode().IsRegular() {
				return current
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

func resolvePromptReasoningLevel(meta *PortalMetadata, reasoningEffort string) string {
	if meta != nil && meta.EmitThinking {
		if strings.TrimSpace(reasoningEffort) != "" {
			return "on"
		}
		return "on"
	}
	return "off"
}
