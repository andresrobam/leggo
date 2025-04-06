package config

func ApplyDefaults(config *Config) {
	config.RefreshMillis = 6
	config.ForceDockerComposeAnsi = true
	config.MaxLogBytes = 10 * 1024 * 1024
	config.InitialLineCapacity = 50
	config.LineCapacityMultiplier = 1.1
	applyOsSpecificDefaults(config)
}
