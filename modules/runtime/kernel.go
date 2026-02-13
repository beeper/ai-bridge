package runtime

import "github.com/beeper/ai-bridge/modules/contracts"

type FeatureModule interface {
	Name() string
	Register(*Kernel) error
}

type Kernel struct {
	Profile   BridgeProfile
	Contracts contracts.CommonConfig
	Modules   []FeatureModule
}

func NewKernel(profile BridgeProfile, common contracts.CommonConfig) *Kernel {
	return &Kernel{Profile: profile, Contracts: common}
}

func (k *Kernel) AddModule(m FeatureModule) error {
	if m == nil {
		return nil
	}
	if err := m.Register(k); err != nil {
		return err
	}
	k.Modules = append(k.Modules, m)
	return nil
}
