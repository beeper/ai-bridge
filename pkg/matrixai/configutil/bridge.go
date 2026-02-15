package configutil

import "go.mau.fi/util/configupgrade"

type BridgeConfig struct {
	CommandPrefix string `yaml:"command_prefix"`
}

func UpgradeBridgeConfig(helper configupgrade.Helper) {
	helper.Copy(configupgrade.Str, "bridge", "command_prefix")
}
