package tools

import "github.com/beeper/ai-bridge/pkg/simpleruntime/agents/toolpolicy"

type ToolType string

type ToolAnnotations struct {
	Title string
}

type Tool struct {
	Name        string
	Description string
	Type        ToolType
	Annotations *ToolAnnotations
	Policy      *toolpolicy.ToolPolicyConfig
	InputSchema any
}

func GetTool(name string) *Tool { _ = name; return nil }
func BuiltinTools() []*Tool { return nil }
func SessionTools() []*Tool { return nil }
func BossTools() []*Tool { return nil }
func IsBossTool(name string) bool { _ = name; return false }
