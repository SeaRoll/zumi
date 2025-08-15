package config_test

import (
	"os"
	"testing"

	"github.com/SeaRoll/zumi/config"
	"github.com/stretchr/testify/assert"
)

type testConfig struct {
	config.BaseConfig `yaml:",inline"`
	NullableStruct    *struct {
		Field string `yaml:"field"`
	} `yaml:"nullable,omitempty"`
}

func (tc testConfig) GetBaseConfig() config.BaseConfig {
	return tc.BaseConfig
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
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Nil(t, cfg.NullableStruct)

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
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "db.example.com", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)

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
	assert.Equal(t, "db.example.com", cfg.NullableStruct.Field)

	t.Logf("Parsed config: %+v", cfg)
	os.Unsetenv("NULL_FIELD")
}
