package parser

import (
	"github.com/arduino/go-paths-helper"
	"gopkg.in/yaml.v3"
)

type Descriptor struct {
	DisplayName        string   `yaml:"display-name"`
	Description        string   `yaml:"description"`
	Ports              []int    `yaml:"ports"`
	ModuleDependencies []string `yaml:"module-dependencies"`
	Models             []string `yaml:"models"`
}

// ParseAppFile reads an app file
func ParseDescriptorFile(file *paths.Path) (Descriptor, error) {
	data, err := file.ReadFile()
	if err != nil {
		return Descriptor{}, err
	}
	app := Descriptor{}
	if err := yaml.Unmarshal(data, &app); err != nil {
		return Descriptor{}, err
	}
	return app, nil
}

func (d *Descriptor) AsYaml() ([]byte, error) {
	res, err := yaml.Marshal(d)
	if err != nil {
		return nil, err
	}
	return res, nil
}
