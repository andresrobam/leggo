package yaml

import (
	"bytes"
	"os"
	"regexp"

	"github.com/goccy/go-yaml"
)

func WithoutExtension(fileName string) string {
	ymlRegex := regexp.MustCompile(`(.*)\.[yY][aA]?[mM][lL]`)
	if ymlRegex.MatchString(fileName) {
		return ymlRegex.ReplaceAllString(fileName, "$1")
	} else {
		return fileName
	}
}

func ImportYamlFile(fileName string, target interface{}) error {
	ymlData, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}
	return ImportYaml(ymlData, target)
}

func ImportYaml(ymlData []byte, target interface{}) error {
	return yaml.Unmarshal(ymlData, &target)
}

func GetKeys(ymlData []byte, yamlPath string) ([]string, error) {
	var yamlMap yaml.MapSlice
	path, err := yaml.PathString(yamlPath)
	if err != nil {
		return nil, err
	}

	path.Read(bytes.NewReader(ymlData), &yamlMap)

	keys := make([]string, len(yamlMap))

	for i := range yamlMap {
		keys[i] = yamlMap[i].Key.(string)
	}

	return keys, nil
}

func GetBytes(source interface{}) ([]byte, error) {
	return yaml.Marshal(source)
}
