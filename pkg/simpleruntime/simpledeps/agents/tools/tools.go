package tools

import (
	"fmt"
	"strconv"
	"strings"
)

type ToolType string

const ResultError ResultStatus = "error"

type ToolAnnotations struct{ Title string }

type Tool struct {
	Name        string
	Description string
	Type        ToolType
	Group       string
	Annotations *ToolAnnotations
	InputSchema any
}

type ResultStatus string

type Result struct {
	Status  ResultStatus
	Content string
}

func (r *Result) Text() string {
	if r == nil {
		return ""
	}
	return r.Content
}

func JSONResult(payload any) *Result           { _ = payload; return &Result{Status: "ok"} }
func ErrorResult(_ string, msg string) *Result { return &Result{Status: ResultError, Content: msg} }

type BossToolExecutor struct{}

type AgentStoreInterface interface{}

type AgentData struct {
	ID           string          `json:"id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Description  string          `json:"description,omitempty"`
	Model        *ModelData      `json:"model,omitempty"`
	Tools        *ToolInfo       `json:"tools,omitempty"`
	SystemPrompt string          `json:"system_prompt,omitempty"`
	Subagents    *SubagentConfig `json:"subagents,omitempty"`
	IsPreset     bool            `json:"is_preset,omitempty"`
}

type ModelData struct {
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name,omitempty"`
	Provider    string   `json:"provider,omitempty"`
	Description string   `json:"description,omitempty"`
	Primary     string   `json:"primary,omitempty"`
	Fallbacks   []string `json:"fallbacks,omitempty"`
}

type ToolInfo struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Type        ToolType `json:"type,omitempty"`
	Group       string   `json:"group,omitempty"`
	Enabled     bool     `json:"enabled,omitempty"`
	Profile     string   `json:"profile,omitempty"`
	Allow       []string `json:"allow,omitempty"`
	Deny        []string `json:"deny,omitempty"`
}

func (t *ToolInfo) Clone() *ToolInfo {
	if t == nil {
		return nil
	}
	out := *t
	if len(t.Allow) > 0 {
		out.Allow = append([]string(nil), t.Allow...)
	}
	if len(t.Deny) > 0 {
		out.Deny = append([]string(nil), t.Deny...)
	}
	return &out
}

type SubagentConfig struct {
	Model       string   `json:"model,omitempty"`
	AllowAgents []string `json:"allowAgents,omitempty"`
}

type RoomData struct {
	ID           string `json:"id,omitempty"`
	RoomID       string `json:"room_id,omitempty"`
	AgentID      string `json:"agent_id,omitempty"`
	Name         string `json:"name,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
}

func NewBossToolExecutor(any) *BossToolExecutor { return &BossToolExecutor{} }
func (b *BossToolExecutor) ExecuteCreateAgent(any, map[string]any) (*Result, error) {
	return ErrorResult("create_agent", "not available in simple bridge"), nil
}
func (b *BossToolExecutor) ExecuteForkAgent(any, map[string]any) (*Result, error) {
	return ErrorResult("fork_agent", "not available in simple bridge"), nil
}
func (b *BossToolExecutor) ExecuteEditAgent(any, map[string]any) (*Result, error) {
	return ErrorResult("edit_agent", "not available in simple bridge"), nil
}
func (b *BossToolExecutor) ExecuteDeleteAgent(any, map[string]any) (*Result, error) {
	return ErrorResult("delete_agent", "not available in simple bridge"), nil
}
func (b *BossToolExecutor) ExecuteListAgents(any, map[string]any) (*Result, error) {
	return ErrorResult("list_agents", "not available in simple bridge"), nil
}
func (b *BossToolExecutor) ExecuteListModels(any, map[string]any) (*Result, error) {
	return ErrorResult("list_models", "not available in simple bridge"), nil
}
func (b *BossToolExecutor) ExecuteRunInternalCommand(any, map[string]any) (*Result, error) {
	return ErrorResult("run_internal_command", "not available in simple bridge"), nil
}
func (b *BossToolExecutor) ExecuteModifyRoom(any, map[string]any) (*Result, error) {
	return ErrorResult("modify_room", "not available in simple bridge"), nil
}

func GetTool(_ string) *Tool                 { return nil }
func BuiltinTools() []*Tool                  { return []*Tool{{Name: "web_search", Description: "Search the web"}} }
func SessionTools() []*Tool                  { return nil }
func BossTools() []*Tool                     { return nil }
func IsBossTool(_ string) bool               { return false }
func IsSessionTool(_ string) bool            { return false }
func IsPluginTool(_ *Tool) bool              { return false }
func PluginIDForTool(_ *Tool) (string, bool) { return "", false }

func ReadStringArray(map[string]any, string) []string   { return nil }

func ReadString(params map[string]any, key string, required bool) (string, error) {
	v, ok := params[key]
	if !ok || v == nil {
		if required {
			return "", fmt.Errorf("parameter %q is required", key)
		}
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		if required {
			return "", fmt.Errorf("parameter %q must be a string", key)
		}
		return "", nil
	}
	return strings.TrimSpace(s), nil
}

func ReadStringDefault(params map[string]any, key, defaultVal string) string {
	s, err := ReadString(params, key, false)
	if err != nil || s == "" {
		return defaultVal
	}
	return s
}

func ReadNumber(params map[string]any, key string, required bool) (float64, error) {
	v, ok := params[key]
	if !ok || v == nil {
		if required {
			return 0, fmt.Errorf("parameter %q is required", key)
		}
		return 0, nil
	}
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			if required {
				return 0, fmt.Errorf("parameter %q must be a number", key)
			}
			return 0, nil
		}
		return f, nil
	}
	if required {
		return 0, fmt.Errorf("parameter %q must be a number", key)
	}
	return 0, nil
}

func ReadInt(params map[string]any, key string, required bool) (int, error) {
	n, err := ReadNumber(params, key, required)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func ReadIntDefault(params map[string]any, key string, defaultVal int) int {
	if _, ok := params[key]; !ok {
		return defaultVal
	}
	n, err := ReadInt(params, key, false)
	if err != nil {
		return defaultVal
	}
	return n
}

func ReadBool(params map[string]any, key string, defaultVal bool) bool {
	v, ok := params[key]
	if !ok {
		return defaultVal
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		lower := strings.ToLower(strings.TrimSpace(b))
		return lower == "true" || lower == "1" || lower == "yes"
	case float64:
		return b != 0
	case int:
		return b != 0
	}
	return defaultVal
}

type Registry struct{}

func DefaultRegistry() *Registry { return &Registry{} }

func (r *Registry) All() []*Tool {
	return BuiltinTools()
}
