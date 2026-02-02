package connector

import "context"

func (oc *AIClient) getModelCapabilitiesForMeta(meta *PortalMetadata) ModelCapabilities {
	modelID := oc.effectiveModel(meta)
	return getModelCapabilities(modelID, oc.findModelInfo(modelID))
}

// getRoomCapabilities returns effective room capabilities, including image-understanding union
// when an agent is assigned and the room is not in raw mode.
func (oc *AIClient) getRoomCapabilities(ctx context.Context, meta *PortalMetadata) ModelCapabilities {
	caps := oc.getModelCapabilitiesForMeta(meta)
	if caps.SupportsVision {
		goto audio
	}
	if !oc.canUseMediaUnderstanding(meta) {
		goto audio
	}
	if oc.resolveImageUnderstandingModel(ctx, meta) != "" {
		caps.SupportsVision = true
	}
audio:
	if caps.SupportsAudio {
		return caps
	}
	if !oc.canUseMediaUnderstanding(meta) {
		return caps
	}
	if oc.resolveAudioUnderstandingModel(ctx, meta) != "" {
		caps.SupportsAudio = true
	}
	return caps
}
