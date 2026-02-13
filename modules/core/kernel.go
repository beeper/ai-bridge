package core

import (
	"github.com/beeper/ai-bridge/modules/contracts"
	"github.com/beeper/ai-bridge/modules/runtime"
)

type FeatureModule = runtime.FeatureModule
type Kernel = runtime.Kernel

func NewKernel(profile runtime.BridgeProfile, common contracts.CommonConfig) *runtime.Kernel {
	return runtime.NewKernel(profile, common)
}
