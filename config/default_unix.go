//go:build unix

package config

func applyOsSpecificDefaults(config *Config) {
	config.CommandExecutor = "/bin/bash"
	config.CommandArgument = "-c"
}
