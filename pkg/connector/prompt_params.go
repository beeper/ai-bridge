package connector

import (
	"os"
	"path/filepath"
	"strings"
)

func resolvePromptWorkspaceDir() string {
	cwd, err := os.Getwd()
	if err == nil && strings.TrimSpace(cwd) != "" {
		return cwd
	}
	return "/"
}

func resolvePromptRepoRoot(workspaceDir string) string {
	workspaceDir = strings.TrimSpace(workspaceDir)
	if workspaceDir == "" {
		return ""
	}
	root := findGitRoot(workspaceDir)
	return root
}

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
