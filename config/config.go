package config

import (
	"os"

	"github.com/andresrobam/leggo/yaml"
)

const configSubDirectory = "/.config/leggo"
const configFile = "/config.yml"
const contextSettingsFile = "/context-settings.yml"

type Config struct {
	RefreshMillis          int
	CommandExecutor        string
	CommandArgument        string
	ForceDockerComposeAnsi bool
	MaxLogBytes            int
}

type ContextSettings struct {
	ServiceOrder  []string
	ActiveService string
}

func WriteContextSettings(contextFilePath *string, contextSettings *ContextSettings) error {

	cs := make(map[string]ContextSettings)
	if err := ReadContextSettings(&cs); err != nil {
		cs = make(map[string]ContextSettings)
	}

	path, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	path += configSubDirectory

	if err := os.MkdirAll(path, 0o0755); err != nil {
		return err
	}

	path += contextSettingsFile

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	cs[*contextFilePath] = *contextSettings
	ymlData, err := yaml.GetBytes(&cs)
	if err != nil {
		return err
	}
	if _, err := file.Write(ymlData); err != nil {
		return err
	}
	return file.Close()
}

func ReadContextSettings(target *map[string]ContextSettings) error {

	path, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := yaml.ImportYamlFile(path+configSubDirectory+contextSettingsFile, target); err != nil {
		return err
	}
	return nil
}

func ReadConfig(config *Config) error {

	path, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return yaml.ImportYamlFile(path+configSubDirectory+configFile, config)
}
