package config_test

import (
	"os"
	"testing"

	"github.com/SeaRoll/zumi/config"
	"github.com/stretchr/testify/assert"
)

type testConfig struct {
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
	Database struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"database"`
	NullableStruct *struct {
		Field string `yaml:"field"`
	} `yaml:"nullable,omitempty"`
}

func TestParseWithNoEnv(t *testing.T) {
	yamlContent := `
server:
  port: 8080
database:
  host: ${DATABASE_HOST:localhost}
  port: ${DATABASE_PORT:5432}
`

	cfg, err := config.FromYAML[testConfig](yamlContent)
	assert.Nil(t, err)
	assert.Equal(t, 8080, cfg.Content.Server.Port)
	assert.Equal(t, "localhost", cfg.Content.Database.Host)
	assert.Equal(t, 5432, cfg.Content.Database.Port)
	assert.Nil(t, cfg.Content.NullableStruct)

	t.Logf("Parsed config: %+v", cfg)
}

func TestParseWithEnv(t *testing.T) {
	yamlContent := `
server:
  port: 8080
database:
  host: ${DATABASE_HOST:localhost}
  port: ${DATABASE_PORT:5432}
`

	os.Setenv("DATABASE_HOST", "db.example.com")

	cfg, err := config.FromYAML[testConfig](yamlContent)
	assert.Nil(t, err)
	assert.Equal(t, 8080, cfg.Content.Server.Port)
	assert.Equal(t, "db.example.com", cfg.Content.Database.Host)
	assert.Equal(t, 5432, cfg.Content.Database.Port)

	t.Logf("Parsed config: %+v", cfg)
	os.Unsetenv("DATABASE_HOST")
}

func TestParseWithNullable(t *testing.T) {
	yamlContent := `
server:
  port: 8080
database:
  host: ${DATABASE_HOST:localhost}
  port: ${DATABASE_PORT:5432}
nullable:
  field: ${NULL_FIELD:default_value}
`

	os.Setenv("NULL_FIELD", "db.example.com")

	cfg, err := config.FromYAML[testConfig](yamlContent)
	assert.Nil(t, err)
	assert.Equal(t, "db.example.com", cfg.Content.NullableStruct.Field)

	t.Logf("Parsed config: %+v", cfg)
	os.Unsetenv("NULL_FIELD")
}
