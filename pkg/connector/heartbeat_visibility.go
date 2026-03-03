package connector

type ResolvedHeartbeatVisibility struct {
	ShowOk       bool
	ShowAlerts   bool
	UseIndicator bool
}

var defaultHeartbeatVisibility = ResolvedHeartbeatVisibility{
	ShowOk:       false,
	ShowAlerts:   true,
	UseIndicator: true,
}

func resolveHeartbeatVisibility(cfg *Config, channel string) ResolvedHeartbeatVisibility {
	if cfg == nil || cfg.Channels == nil {
		return defaultHeartbeatVisibility
	}
	result := defaultHeartbeatVisibility
	if d := cfg.Channels.Defaults; d != nil {
		applyVisibilityOverrides(&result, d.Heartbeat)
	}
	if (channel == "" || channel == "matrix") && cfg.Channels.Matrix != nil {
		applyVisibilityOverrides(&result, cfg.Channels.Matrix.Heartbeat)
	}
	return result
}

// applyVisibilityOverrides merges non-nil visibility fields into result.
func applyVisibilityOverrides(result *ResolvedHeartbeatVisibility, hb *ChannelHeartbeatVisibilityConfig) {
	if hb == nil {
		return
	}
	if hb.ShowOk != nil {
		result.ShowOk = *hb.ShowOk
	}
	if hb.ShowAlerts != nil {
		result.ShowAlerts = *hb.ShowAlerts
	}
	if hb.UseIndicator != nil {
		result.UseIndicator = *hb.UseIndicator
	}
}
