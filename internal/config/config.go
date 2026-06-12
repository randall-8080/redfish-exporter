package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Modules map[string]Module `yaml:"modules"`
}

type Module struct {
	Collectors CollectorsConfig `yaml:"collectors"`
	Auth       Auth             `yaml:"auth"`
	Insecure   bool             `yaml:"insecure"`
}

type Auth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type CollectorsConfig struct {
	Thermal bool `yaml:"thermal"`
	Power   bool `yaml:"power"`
	Systems bool `yaml:"systems"`
	Memory  bool `yaml:"memory"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if _, ok := c.Modules["default"]; !ok {
		return nil, fmt.Errorf("default module missing")
	}
	return &c, nil
}
