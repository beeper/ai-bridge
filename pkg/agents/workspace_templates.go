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
	path := "workspace_templates/" + name
	data, err := workspaceTemplates.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("missing workspace template: %s", name)
	}
	return stripFrontMatter(string(data)), nil
}
