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
	LogBytes               int
}

type ContextSettings struct {
	ServiceOrder []string
}

type ContextSettingsMap map[string]ContextSettings

func WriteContextSettings(contextFilePath *string, contextSettings *ContextSettings) error {

	cs := ContextSettingsMap{}
	if err := ReadContextSettings(&cs); err != nil {
		cs = ContextSettingsMap{}
	}

	path, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	path += configSubDirectory

	if err := os.MkdirAll(path, 0o0666); err != nil {
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
	_, err = file.Write(ymlData)
	return err
}

func ReadContextSettings(target *ContextSettingsMap) error {

	path, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := yaml.ImportYamlFile(path+configSubDirectory+contextSettingsFile, target); err != nil {
		return err
	}
	return nil
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
