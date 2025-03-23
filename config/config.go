package config

import (
	"os"

	"github.com/andresrobam/leggo/yaml"
)

const configSubDirectory = "/.config/leggo"
const configFile = "/config.yml"

type Config struct {
	RefreshMillis          int
	CommandExecutor        string
	CommandArgument        string
	ForceDockerComposeAnsi bool
	LogBytes               int
	ContextSettings        map[string]ContextSettings
}

type ContextSettings struct {
	ServiceOrder []string
}

func WriteConfig(config *Config) error {

	path, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	path += configSubDirectory

	if err := os.MkdirAll(path, 0o0666); err != nil {
		return err
	}

	file, err := os.Create(path + configFile)
	if err != nil {
		return err
	}
	ymlData, err := yaml.GetBytes(*config)
	if err != nil {
		return err
	}
	_, err = file.Write(ymlData)
	return err
}

func ReadConfig() (*Config, error) {

	path, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.ImportYamlFile(path+configSubDirectory+configFile, config); err != nil {
		return nil, err
	}

	return &config, nil
}
