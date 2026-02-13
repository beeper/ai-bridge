package toolpolicy

type ToolPolicyProfile string

const (
	ProfileFull ToolPolicyProfile = "full"
)

type ToolPolicyConfig struct {
	Profile ToolPolicyProfile `json:"profile,omitempty"`
}

type GlobalToolPolicyConfig struct{}

func (c *ToolPolicyConfig) Clone() *ToolPolicyConfig {
	if c == nil {
		return nil
	}
	clone := *c
	return &clone
}

func (c *GlobalToolPolicyConfig) Clone() *GlobalToolPolicyConfig {
	if c == nil {
		return nil
	}
	clone := *c
	return &clone
}
