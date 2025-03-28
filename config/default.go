package config

func ApplyDefaults(config *Config) {
	config.RefreshMillis = 6
	config.ForceDockerComposeAnsi = true
	config.MaxLogBytes = 10 * 1024 * 1024
	applyOsSpecificDefaults(config)
}
