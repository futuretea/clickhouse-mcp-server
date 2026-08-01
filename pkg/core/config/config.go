package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

// StaticConfig contains configuration that is fixed when the server starts.
type StaticConfig struct {
	Port            int              `mapstructure:"port"`
	Listen          string           `mapstructure:"listen"`
	SSEBaseURL      string           `mapstructure:"sse_base_url"`
	LogLevel        string           `mapstructure:"log_level"`
	EnabledTools    []string         `mapstructure:"enabled_tools"`
	DisabledTools   []string         `mapstructure:"disabled_tools"`
	EnabledDomains  []string         `mapstructure:"enabled_domains"`
	DisabledDomains []string         `mapstructure:"disabled_domains"`
	ClickHouse      ClickHouseConfig `mapstructure:"clickhouse"`
}

// ClickHouseConfig contains the HTTP(S) connection settings for ClickHouse.
type ClickHouseConfig struct {
	Address  string `mapstructure:"address"`
	Database string `mapstructure:"database"`
	Secure   bool   `mapstructure:"secure"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// Validate checks whether the configuration can be used to start the server.
func (c *StaticConfig) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535, got %d", c.Port)
	}
	if c.Port != 0 && strings.TrimSpace(c.Listen) == "" {
		return fmt.Errorf("listen must be set when port is non-zero")
	}
	if _, err := zerolog.ParseLevel(c.LogLevel); err != nil {
		return fmt.Errorf("invalid log_level %q: %w", c.LogLevel, err)
	}
	if err := c.ClickHouse.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate checks whether the ClickHouse address can be used with HTTP(S).
func (c ClickHouseConfig) Validate() error {
	address := strings.TrimSpace(c.Address)
	if address == "" {
		return fmt.Errorf("clickhouse.address is required; set a host:port such as 127.0.0.1:8123")
	}
	if strings.Contains(address, "://") {
		return fmt.Errorf("clickhouse.address must be host:port without a URL scheme")
	}
	if strings.ContainsAny(address, "/?@") {
		return fmt.Errorf("clickhouse.address must be a host:port: %q", address)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(host) == "" || strings.ContainsAny(host, "/@?") {
		return fmt.Errorf("clickhouse.address must be a host:port: %q", address)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("clickhouse.address must use a port from 1 to 65535")
	}
	return nil
}

// GetListenAddress returns the configured HTTP listen address (host:port).
func (c *StaticConfig) GetListenAddress() string {
	if c.Port == 0 {
		return ""
	}
	return net.JoinHostPort(c.Listen, strconv.Itoa(c.Port))
}

// LoadConfig loads configuration from defaults, optional YAML, environment, and flags.
func LoadConfig(configPath string, v *viper.Viper) (*StaticConfig, error) {
	if v == nil {
		v = viper.New()
	}

	defaults := map[string]any{
		"port":                0,
		"listen":              "127.0.0.1",
		"sse_base_url":        "",
		"log_level":           "info",
		"enabled_tools":       []string{},
		"disabled_tools":      []string{},
		"enabled_domains":     []string{},
		"disabled_domains":    []string{},
		"clickhouse.address":  "",
		"clickhouse.database": "default",
		"clickhouse.secure":   false,
		"clickhouse.username": "",
		"clickhouse.password": "",
	}
	for key, value := range defaults {
		v.SetDefault(key, value)
	}

	if configPath != "" {
		v.SetConfigFile(configPath)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
	}

	v.SetEnvPrefix("MCP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	cfg := &StaticConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	cfg.ClickHouse.Address = strings.TrimSpace(cfg.ClickHouse.Address)
	cfg.ClickHouse.Database = strings.TrimSpace(cfg.ClickHouse.Database)
	cfg.ClickHouse.Username = strings.TrimSpace(cfg.ClickHouse.Username)
	if cfg.ClickHouse.Database == "" {
		cfg.ClickHouse.Database = "default"
	}
	cfg.EnabledTools = normalizeStringSlice(cfg.EnabledTools)
	cfg.DisabledTools = normalizeStringSlice(cfg.DisabledTools)
	cfg.EnabledDomains = normalizeStringSlice(cfg.EnabledDomains)
	cfg.DisabledDomains = normalizeStringSlice(cfg.DisabledDomains)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func normalizeStringSlice(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}
