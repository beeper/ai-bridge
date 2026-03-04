package agents

import (
	"embed"
	"fmt"
	"strings"
	"unicode"
)

//go:embed workspace_templates/*
var workspaceTemplates embed.FS

func stripFrontMatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	endIndex := strings.Index(content[3:], "\n---")
	if endIndex == -1 {
		return content
	}
	endIndex += 3
	start := endIndex + len("\n---")
	trimmed := content[start:]
	return strings.TrimLeftFunc(trimmed, unicode.IsSpace)
}

func loadWorkspaceTemplate(name string) (string, error) {
	data, err := workspaceTemplates.ReadFile("workspace_templates/" + name)
	if err != nil {
		return "", fmt.Errorf("loading workspace template %s: %w", name, err)
	}
	return stripFrontMatter(string(data)), nil
}
