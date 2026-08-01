package config_test

import (
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
)

func TestLoadConfigDefaults(t *testing.T) {
	v := viper.New()
	v.Set("clickhouse.address", "127.0.0.1:8123")
	cfg, err := config.LoadConfig("", v)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Port != 0 {
		t.Fatalf("expected port 0, got %d", cfg.Port)
	}
	if cfg.Listen != "127.0.0.1" {
		t.Fatalf("expected listen 127.0.0.1, got %q", cfg.Listen)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected log_level info, got %q", cfg.LogLevel)
	}
	if cfg.ClickHouse.Database != "default" || cfg.ClickHouse.Secure {
		t.Fatalf("unexpected ClickHouse defaults: %+v", cfg.ClickHouse)
	}
}

func TestLoadConfigRequiresClickHouseAddress(t *testing.T) {
	_, err := config.LoadConfig("", viper.New())
	if err == nil || !strings.Contains(err.Error(), "clickhouse.address is required") {
		t.Fatalf("LoadConfig error = %v", err)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.StaticConfig
		wantSub string
	}{
		{
			name:    "bad port",
			cfg:     config.StaticConfig{Port: 70000, LogLevel: "info", Listen: "127.0.0.1"},
			wantSub: "port must be between",
		},
		{
			name:    "empty listen",
			cfg:     config.StaticConfig{Port: 8080, LogLevel: "info", Listen: " "},
			wantSub: "listen must be set",
		},
		{
			name:    "bad log level",
			cfg:     config.StaticConfig{Port: 0, LogLevel: "nope", Listen: "127.0.0.1"},
			wantSub: "invalid log_level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantSub)
			}
		})
	}
}

func TestClickHouseAddressValidation(t *testing.T) {
	tests := []struct {
		name    string
		address string
		wantErr string
	}{
		{name: "hostname", address: "clickhouse.local:8123"},
		{name: "ipv6", address: "[::1]:8123"},
		{name: "scheme", address: "http://localhost:8123", wantErr: "without a URL scheme"},
		{name: "path", address: "localhost:8123/query", wantErr: "host:port"},
		{name: "missing port", address: "localhost", wantErr: "host:port"},
		{name: "bad port", address: "localhost:70000", wantErr: "port from 1 to 65535"},
		{name: "userinfo", address: "alice@localhost:8123", wantErr: "host:port"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := (config.ClickHouseConfig{Address: testCase.address}).Validate()
			if testCase.wantErr == "" && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if testCase.wantErr != "" && (err == nil || !strings.Contains(err.Error(), testCase.wantErr)) {
				t.Fatalf("Validate error = %v, want %q", err, testCase.wantErr)
			}
		})
	}
}

func TestGetListenAddress(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.StaticConfig
		want string
	}{
		{name: "stdio", cfg: config.StaticConfig{Port: 0, Listen: "127.0.0.1"}, want: ""},
		{name: "IPv4", cfg: config.StaticConfig{Port: 8080, Listen: "127.0.0.1"}, want: "127.0.0.1:8080"},
		{name: "hostname", cfg: config.StaticConfig{Port: 8080, Listen: "localhost"}, want: "localhost:8080"},
		{name: "IPv6", cfg: config.StaticConfig{Port: 8080, Listen: "::1"}, want: "[::1]:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.GetListenAddress(); got != tt.want {
				t.Fatalf("GetListenAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}
