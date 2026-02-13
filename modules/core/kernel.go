package core

import "github.com/beeper/ai-bridge/modules/contracts"

type BridgeKind string

const (
	BridgeSimple  BridgeKind = "simple"
	BridgeAgentic BridgeKind = "agentic"
)

type FeatureModule interface {
	Name() string
	Register(*Kernel) error
}

type Kernel struct {
	Kind      BridgeKind
	Contracts contracts.CommonConfig
	Modules   []FeatureModule
}

func NewKernel(kind BridgeKind, common contracts.CommonConfig) *Kernel {
	return &Kernel{Kind: kind, Contracts: common}
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
