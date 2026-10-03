package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

type ConfigError struct {
	Field    string
	Message  string
	Expected string
	Actual   string
}

func (e *ConfigError) Error() string {
	msg := fmt.Sprintf("config error in field '%s': %s", e.Field, e.Message)
	if e.Expected != "" || e.Actual != "" {
		msg += fmt.Sprintf(" (expected: %s, actual: %s)", e.Expected, e.Actual)
	}
	return msg
}

type ClickHouseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	// Protocol selects the transport: "native" (default, port 9000) or "http" (port 8123).
	Protocol string `yaml:"protocol"`
}

// DefaultPort returns the conventional default port for the configured protocol.
func (c *ClickHouseConfig) DefaultPort() int {
	if c.Protocol == "http" {
		return 8123
	}
	return 9000
}

func (c *ClickHouseConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c *ClickHouseConfig) DSN() string {
	scheme := "clickhouse"
	if c.Protocol == "http" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s:%s@%s/%s", scheme, c.User, c.Password, c.Addr(), c.Database)
}

type StandbyConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Interval string `yaml:"interval"`
}

type RetryConfig struct {
	Enabled     bool   `yaml:"enabled"`
	MaxAttempts int    `yaml:"max_attempts"`
	Backoff     string `yaml:"backoff"`
}

type SyncConfig struct {
	BatchSize      int           `yaml:"batch_size"`
	MaxConcurrency int           `yaml:"max_concurrency"`
	Standby        StandbyConfig `yaml:"standby"`
	Retries        RetryConfig   `yaml:"retries"`
}

type LoggingConfig struct {
	Level     string `yaml:"level"`
	Directory string `yaml:"directory"`
	Console   bool   `yaml:"console"`
}

type TableConfig struct {
	Name       string `yaml:"name"`
	DateColumn string `yaml:"date_column"`
}

type Config struct {
	Source      ClickHouseConfig `yaml:"source"`
	Destination ClickHouseConfig `yaml:"destination"`
	Sync        SyncConfig       `yaml:"sync"`
	Logging     LoggingConfig    `yaml:"logging"`
	Tables      []TableConfig    `yaml:"tables"`
	Timezone    string           `yaml:"timezone"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	c := &Config{
		Sync: SyncConfig{
			BatchSize:      50000,
			MaxConcurrency: 4,
			Retries: RetryConfig{
				MaxAttempts: 3,
				Backoff:     "5s",
			},
		},
		Logging: LoggingConfig{
			Level:     "info",
			Directory: "./logs",
			Console:   true,
		},
		Timezone: "Asia/Kolkata",
		Source:      ClickHouseConfig{},
		Destination: ClickHouseConfig{},
	}

	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("failed to parse yaml: %w", err)
	}

	// Apply protocol defaults before port defaults so DefaultPort() is correct.
	if c.Source.Protocol == "" {
		c.Source.Protocol = "native"
	}
	if c.Destination.Protocol == "" {
		c.Destination.Protocol = "native"
	}

	if c.Source.Port == 0 {
		c.Source.Port = c.Source.DefaultPort()
	}
	if c.Destination.Port == 0 {
		c.Destination.Port = c.Destination.DefaultPort()
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	return c, nil
}

var (
	tableNameRegex  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)
	columnNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

var validProtocols = map[string]bool{"native": true, "http": true}

func (c *Config) Validate() error {
	if c.Source.Host == "" {
		return &ConfigError{Field: "source.host", Message: "required field missing"}
	}
	if c.Destination.Host == "" {
		return &ConfigError{Field: "destination.host", Message: "required field missing"}
	}

	if !validProtocols[c.Source.Protocol] {
		return &ConfigError{Field: "source.protocol", Message: "invalid protocol", Expected: "native/http", Actual: c.Source.Protocol}
	}
	if !validProtocols[c.Destination.Protocol] {
		return &ConfigError{Field: "destination.protocol", Message: "invalid protocol", Expected: "native/http", Actual: c.Destination.Protocol}
	}

	if c.Source.Port < 1 || c.Source.Port > 65535 {
		return &ConfigError{Field: "source.port", Message: "invalid port", Expected: "1-65535", Actual: fmt.Sprintf("%d", c.Source.Port)}
	}
	if c.Destination.Port < 1 || c.Destination.Port > 65535 {
		return &ConfigError{Field: "destination.port", Message: "invalid port", Expected: "1-65535", Actual: fmt.Sprintf("%d", c.Destination.Port)}
	}

	if c.Sync.BatchSize <= 0 || c.Sync.BatchSize > 10000000 {
		return &ConfigError{Field: "sync.batch_size", Message: "invalid batch size", Expected: ">0 and <=10000000", Actual: fmt.Sprintf("%d", c.Sync.BatchSize)}
	}

	if c.Sync.MaxConcurrency <= 0 || c.Sync.MaxConcurrency > 64 {
		return &ConfigError{Field: "sync.max_concurrency", Message: "invalid max concurrency", Expected: ">0 and <=64", Actual: fmt.Sprintf("%d", c.Sync.MaxConcurrency)}
	}

	if c.Sync.Standby.Enabled {
		d, err := time.ParseDuration(c.Sync.Standby.Interval)
		if err != nil || d <= 0 {
			return &ConfigError{Field: "sync.standby.interval", Message: "invalid duration", Expected: "positive duration string (e.g. 1m)", Actual: c.Sync.Standby.Interval}
		}
	}

	if c.Sync.Retries.Enabled {
		d, err := time.ParseDuration(c.Sync.Retries.Backoff)
		if err != nil || d <= 0 {
			return &ConfigError{Field: "sync.retries.backoff", Message: "invalid duration", Expected: "positive duration string (e.g. 5s)", Actual: c.Sync.Retries.Backoff}
		}
	}

	if c.Source.Host == c.Destination.Host && c.Source.Port == c.Destination.Port && c.Source.Database == c.Destination.Database {
		return &ConfigError{Field: "destination", Message: "source and destination cannot be identical"}
	}

	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return &ConfigError{Field: "timezone", Message: "invalid timezone", Expected: "valid IANA timezone", Actual: c.Timezone}
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Logging.Level] {
		return &ConfigError{Field: "logging.level", Message: "invalid logging level", Expected: "debug/info/warn/error", Actual: c.Logging.Level}
	}

	tableNames := make(map[string]bool)
	for i, t := range c.Tables {
		if !tableNameRegex.MatchString(t.Name) {
			return &ConfigError{Field: fmt.Sprintf("tables[%d].name", i), Message: "invalid table name format", Actual: t.Name}
		}
		if t.DateColumn == "" {
			return &ConfigError{Field: fmt.Sprintf("tables[%d].date_column", i), Message: "missing date column"}
		}
		if !columnNameRegex.MatchString(t.DateColumn) {
			return &ConfigError{Field: fmt.Sprintf("tables[%d].date_column", i), Message: "invalid date column format", Actual: t.DateColumn}
		}
		if tableNames[t.Name] {
			return &ConfigError{Field: fmt.Sprintf("tables[%d].name", i), Message: "duplicate table entry", Actual: t.Name}
		}
		tableNames[t.Name] = true
	}

	return nil
}

func (c *Config) GetTimezone() (*time.Location, error) {
	return time.LoadLocation(c.Timezone)
}

func (c *Config) GetStandbyInterval() (time.Duration, error) {
	if !c.Sync.Standby.Enabled {
		return 0, nil
	}
	return time.ParseDuration(c.Sync.Standby.Interval)
}

func (c *Config) GetRetryBackoff() (time.Duration, error) {
	if !c.Sync.Retries.Enabled {
		return 0, nil
	}
	return time.ParseDuration(c.Sync.Retries.Backoff)
}

func (c *Config) FindTable(name string) (*TableConfig, bool) {
	for i := range c.Tables {
		if c.Tables[i].Name == name {
			return &c.Tables[i], true
		}
	}
	return nil, false
}
