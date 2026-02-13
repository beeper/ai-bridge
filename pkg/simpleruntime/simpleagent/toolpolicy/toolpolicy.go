package toolpolicy

type ToolProfileID string

const ProfileFull ToolProfileID = "full"

type ToolPolicy struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

type ToolPolicyConfig struct {
	Profile ToolProfileID `json:"profile,omitempty"`
	Allow   []string      `json:"allow,omitempty"`
	Deny    []string      `json:"deny,omitempty"`
}

type GlobalToolPolicyConfig struct {
	Allow map[string]*ToolPolicy `json:"allow,omitempty"`
}

type PluginToolGroups map[string][]string

func (c *ToolPolicyConfig) Clone() *ToolPolicyConfig {
	if c == nil {
		return nil
	}
	out := *c
	return &out
}
func NormalizeToolName(name string) string                       { return name }
func NormalizeToolList(in []string) []string                     { return in }
func FilterToolsByPolicy(names []string, _ *ToolPolicy) []string { return names }
func StripPluginOnlyAllowlist(policy *ToolPolicy, _ PluginToolGroups, _ map[string]struct{}) (bool, []string, *ToolPolicy) {
	return false, nil, policy
}
func ExpandPolicyWithPluginGroups(policy *ToolPolicy, _ PluginToolGroups) *ToolPolicy { return policy }
func BuildPluginToolGroups[T any](_ []T, _ func(T) string, _ func(T) (string, bool)) PluginToolGroups {
	return PluginToolGroups{}
}
func ResolveToolProfilePolicy(_ ToolProfileID) *ToolPolicy            { return nil }
func MergeAlsoAllow(policy *ToolPolicy, _ []string) *ToolPolicy       { return policy }
func ResolveSubagentToolPolicy(_ *GlobalToolPolicyConfig) *ToolPolicy { return nil }

type EffectiveToolPolicy struct {
	Profile              ToolProfileID
	ProviderProfile      ToolProfileID
	ProfileAlsoAllow     []string
	ProviderAlsoAllow    []string
	GlobalPolicy         *ToolPolicy
	GlobalProviderPolicy *ToolPolicy
	AgentPolicy          *ToolPolicy
	AgentProviderPolicy  *ToolPolicy
}

func ResolveEffectiveToolPolicy(_ any) EffectiveToolPolicy { return EffectiveToolPolicy{} }

func IsOwnerOnlyToolName(_ string) bool { return false }
