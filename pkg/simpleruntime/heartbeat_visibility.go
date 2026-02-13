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

	defaults := cfg.Channels.Defaults
	perChannel := cfg.Channels.Matrix
	if channel != "" && channel != "matrix" {
		perChannel = nil
	}

	result := ResolvedHeartbeatVisibility{
		ShowOk:       defaultHeartbeatVisibility.ShowOk,
		ShowAlerts:   defaultHeartbeatVisibility.ShowAlerts,
		UseIndicator: defaultHeartbeatVisibility.UseIndicator,
	}

	if defaults != nil && defaults.Heartbeat != nil {
		if defaults.Heartbeat.ShowOk != nil {
			result.ShowOk = *defaults.Heartbeat.ShowOk
		}
		if defaults.Heartbeat.ShowAlerts != nil {
			result.ShowAlerts = *defaults.Heartbeat.ShowAlerts
		}
		if defaults.Heartbeat.UseIndicator != nil {
			result.UseIndicator = *defaults.Heartbeat.UseIndicator
		}
	}

	if perChannel != nil && perChannel.Heartbeat != nil {
		if perChannel.Heartbeat.ShowOk != nil {
			result.ShowOk = *perChannel.Heartbeat.ShowOk
		}
		if perChannel.Heartbeat.ShowAlerts != nil {
			result.ShowAlerts = *perChannel.Heartbeat.ShowAlerts
		}
		if perChannel.Heartbeat.UseIndicator != nil {
			result.UseIndicator = *perChannel.Heartbeat.UseIndicator
		}
	}

	return result
}
