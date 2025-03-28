//go:build windows

package config

func applyOsSpecificDefaults(config *Config) {
	config.CommandExecutor = "cmd"
	config.CommandArgument = "/C"
}
