package connector

import "github.com/beeper/ai-bridge/pkg/core/aimodels"

func toCoreModelInfo(info *ModelInfo) *aimodels.ModelInfo {
	if info == nil {
		return nil
	}
	converted := aimodels.ModelInfo(*info)
	return &converted
}

func fromCoreModelInfo(info *aimodels.ModelInfo) *ModelInfo {
	if info == nil {
		return nil
	}
	converted := ModelInfo(*info)
	return &converted
}
