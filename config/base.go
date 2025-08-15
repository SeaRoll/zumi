package config

type ZumiConfig interface {
	GetBaseConfig() BaseConfig
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type DatabaseConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
}

type PubsubConfig struct {
	Enabled       bool   `yaml:"enabled"` // Whether Pubsub is enabled
	ConnectionUrl string `yaml:"url"`     // NATS server connection URL
	Name          string `yaml:"name"`    // Name of the JetStream stream
	TopicPrefix   string `yaml:"prefix"`  // Prefix for topics in the stream
	MaxAge        string `yaml:"maxAge"`  // Maximum age of messages in the stream
}

type SentinelOption struct {
	Enabled   bool   `yaml:"enabled"`   // Whether Sentinel is enabled
	MasterSet string `yaml:"masterSet"` // MasterSet is the name of the master set for sentinel mode
	Password  string `yaml:"password"`  // Password for the sentinel, if not provided, it will not use sentinel
}

type CacheConfig struct {
	Enabled        bool           `yaml:"enabled"`  // Whether Cache is enabled
	Host           string         `yaml:"host"`     // Host of the cache server
	Port           string         `yaml:"port"`     // Port of the cache server
	Password       string         `yaml:"password"` // Password for the cache server
	SentinelConfig SentinelOption `yaml:"sentinel"` // SentinelConfig for sentinel mode, if nil, it will not use sentinel
}

type BaseConfig struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Pubsub   PubsubConfig   `yaml:"pubsub"`
	Cache    CacheConfig    `yaml:"cache"`
}

func (bc BaseConfig) GetBaseConfig() BaseConfig {
	return bc
}
