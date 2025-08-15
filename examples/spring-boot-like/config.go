package springbootlike

import (
	_ "embed"
	"fmt"

	"github.com/SeaRoll/zumi/config"
)

type AppConfig struct {
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
	Database struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
		Name string `yaml:"name"`
		User string `yaml:"user"`
		Pass string `yaml:"pass"`
	} `yaml:"database"`
}

//go:embed config.yaml
var configData string

func LoadConfig() (AppConfig, error) {
	// Parse the YAML configuration
	cfg, err := config.FromYAML[AppConfig](configData)
	if err != nil {
		return cfg.Content, fmt.Errorf("failed to load config: %w", err)
	}

	// Return the parsed configuration
	return cfg.Content, nil
}
