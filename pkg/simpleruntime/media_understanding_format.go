package connector

import "github.com/beeper/ai-bridge/pkg/core/aimedia"

func formatMediaUnderstandingBody(body string, outputs []MediaUnderstandingOutput) string {
	return aimedia.FormatMediaUnderstandingBody(body, toCoreMediaOutputs(outputs))
}

func formatAudioTranscripts(outputs []MediaUnderstandingOutput) string {
	return aimedia.FormatAudioTranscripts(toCoreMediaOutputs(outputs))
}
