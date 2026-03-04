package connector

import "testing"

func TestPkgAIRuntimeEnabledFromEnv(t *testing.T) {
	t.Setenv("PI_USE_PKG_AI_RUNTIME", "")
	if pkgAIRuntimeEnabled() {
		t.Fatalf("expected runtime flag disabled by default")
	}

	t.Setenv("PI_USE_PKG_AI_RUNTIME", "1")
	if !pkgAIRuntimeEnabled() {
		t.Fatalf("expected runtime flag enabled for value 1")
	}

	t.Setenv("PI_USE_PKG_AI_RUNTIME", "true")
	if !pkgAIRuntimeEnabled() {
		t.Fatalf("expected runtime flag enabled for value true")
	}

	t.Setenv("PI_USE_PKG_AI_RUNTIME", "off")
	if pkgAIRuntimeEnabled() {
		t.Fatalf("expected runtime flag disabled for value off")
	}
}

func TestChooseStreamingRuntimePath(t *testing.T) {
	if got := chooseStreamingRuntimePath(true, ModelAPIResponses, true); got != streamingRuntimeChatCompletions {
		t.Fatalf("expected audio to force chat completions, got %s", got)
	}
	if got := chooseStreamingRuntimePath(false, ModelAPIResponses, true); got != streamingRuntimePkgAI {
		t.Fatalf("expected pkg_ai path when preferred and no audio, got %s", got)
	}
	if got := chooseStreamingRuntimePath(false, ModelAPIChatCompletions, false); got != streamingRuntimeChatCompletions {
		t.Fatalf("expected chat model api path, got %s", got)
	}
	if got := chooseStreamingRuntimePath(false, ModelAPIResponses, false); got != streamingRuntimeResponses {
		t.Fatalf("expected responses path fallback, got %s", got)
	}
}
