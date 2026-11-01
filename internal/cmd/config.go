package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
)

type clickHouseProbe interface {
	Ping(context.Context) error
	Close() error
}

var openClickHouseProbe = func(options *ch.Options) (clickHouseProbe, error) {
	connection, err := ch.Open(options)
	if err != nil {
		return nil, err
	}
	return connection, nil
}

type checkField struct {
	name   string
	value  string
	source string
}

func newConfigCommand(streams IOStreams, cfgFile *string, v *viper.Viper) *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "inspect the effective configuration",
	}
	command.AddCommand(newConfigCheckCommand(streams, cfgFile, v))
	return command
}

func newConfigCheckCommand(streams IOStreams, cfgFile *string, v *viper.Viper) *cobra.Command {
	command := &cobra.Command{
		Use:   "check",
		Short: "display the effective configuration without connecting",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadCLIConfig(cmd, *cfgFile, v)
			if err != nil {
				return safeConfigLoadError(err)
			}

			if err := writeConfigCheck(cmd.OutOrStdout(), configCheckFields(cmd, v, cfg)); err != nil {
				return err
			}
			if err := writeConfigPath(cmd.OutOrStdout(), cmd.Root(), *cfgFile, cfg.ClickHouse); err != nil {
				return err
			}
			connect, err := cmd.Flags().GetBool("connect")
			if err != nil {
				return fmt.Errorf("read connect flag: %w", err)
			}
			if !connect {
				return nil
			}
			if err := verifyClickHouse(cmd.Context(), cfg.ClickHouse); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "ClickHouse connection verified.")
			return err
		},
	}
	command.Flags().Bool("connect", false, "verify the configured ClickHouse connection")
	return command
}

func configCheckFields(cmd *cobra.Command, v *viper.Viper, cfg *config.StaticConfig) []checkField {
	fields := []checkField{
		{name: "clickhouse.address", value: cfg.ClickHouse.Address, source: configValueSource(cmd, v, "clickhouse.address", "MCP_CLICKHOUSE_ADDRESS", "")},
		{name: "clickhouse.database", value: cfg.ClickHouse.Database, source: configValueSource(cmd, v, "clickhouse.database", "MCP_CLICKHOUSE_DATABASE", cfg.ClickHouse.Database)},
		{name: "clickhouse.secure", value: strconv.FormatBool(cfg.ClickHouse.Secure), source: configValueSource(cmd, v, "clickhouse.secure", "MCP_CLICKHOUSE_SECURE", "")},
		{name: "clickhouse.username", value: cfg.ClickHouse.Username, source: configValueSource(cmd, v, "clickhouse.username", "MCP_CLICKHOUSE_USERNAME", "")},
		{name: "clickhouse.password_status", value: passwordStatus(cfg.ClickHouse.Password), source: configValueSource(cmd, v, "clickhouse.password", "MCP_CLICKHOUSE_PASSWORD", "")},
		{name: "port", value: strconv.Itoa(cfg.Port), source: configValueSource(cmd, v, "port", "MCP_PORT", "")},
		{name: "listen", value: cfg.Listen, source: configValueSource(cmd, v, "listen", "MCP_LISTEN", "")},
		{name: "sse_base_url", value: safeSSEBaseURL(cfg.SSEBaseURL), source: configValueSource(cmd, v, "sse_base_url", "MCP_SSE_BASE_URL", "")},
		{name: "log_level", value: cfg.LogLevel, source: configValueSource(cmd, v, "log_level", "MCP_LOG_LEVEL", "")},
		{name: "enabled_tools", value: strings.Join(cfg.EnabledTools, ","), source: configValueSource(cmd, v, "enabled_tools", "MCP_ENABLED_TOOLS", "")},
		{name: "disabled_tools", value: strings.Join(cfg.DisabledTools, ","), source: configValueSource(cmd, v, "disabled_tools", "MCP_DISABLED_TOOLS", "")},
		{name: "enabled_domains", value: strings.Join(cfg.EnabledDomains, ","), source: configValueSource(cmd, v, "enabled_domains", "MCP_ENABLED_DOMAINS", "")},
		{name: "disabled_domains", value: strings.Join(cfg.DisabledDomains, ","), source: configValueSource(cmd, v, "disabled_domains", "MCP_DISABLED_DOMAINS", "")},
		{name: "allow_unsafe_database_role", value: strconv.FormatBool(cfg.AllowUnsafeDatabaseRole), source: configValueSource(cmd, v, "allow_unsafe_database_role", "MCP_ALLOW_UNSAFE_DATABASE_ROLE", "")},
	}
	for index := range fields {
		if fields[index].name == "clickhouse.password_status" {
			continue
		}
		fields[index].value = redactPasswordForms(fields[index].value, cfg.ClickHouse.Username, cfg.ClickHouse.Password)
	}
	return fields
}

func configValueSource(cmd *cobra.Command, v *viper.Viper, key, environmentKey, value string) string {
	if flagName, ok := configFlagNames[key]; ok && cmd.Root().PersistentFlags().Changed(flagName) {
		return "flag"
	}
	if environmentValue, ok := os.LookupEnv(environmentKey); ok && environmentValue != "" {
		if key == "clickhouse.database" && value == "default" && strings.TrimSpace(environmentValue) == "" {
			return "default"
		}
		return "environment"
	}
	if v.InConfig(key) {
		if key == "clickhouse.database" && value == "default" && strings.TrimSpace(v.GetString(key)) == "" {
			return "default"
		}
		return "YAML"
	}
	return "default"
}

var configFlagNames = map[string]string{
	"enabled_tools":    "enabled-tools",
	"disabled_tools":   "disabled-tools",
	"enabled_domains":  "enable-domains",
	"disabled_domains": "disable-domains",
}

func passwordStatus(password string) string {
	if password == "" {
		return "unset"
	}
	return "set"
}

func safeSSEBaseURL(value string) string {
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "configured"
	}
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.Redacted()
}

func writeConfigCheck(output interface{ Write([]byte) (int, error) }, fields []checkField) error {
	for _, field := range fields {
		if _, err := fmt.Fprintf(output, "%s=%s source=%s\n", field.name, field.value, field.source); err != nil {
			return err
		}
	}
	return nil
}

func writeConfigPath(output interface{ Write([]byte) (int, error) }, root *cobra.Command, cfgFile string, clickHouse config.ClickHouseConfig) error {
	source := "default"
	if root.PersistentFlags().Changed("config") {
		source = "flag"
	}
	if cfgFile == "" {
		cfgFile = "(not set)"
	}
	cfgFile = redactPasswordForms(cfgFile, clickHouse.Username, clickHouse.Password)
	_, err := fmt.Fprintf(output, "config=%s source=%s\n", cfgFile, source)
	return err
}

func safeConfigLoadError(err error) error {
	message := err.Error()
	if strings.HasPrefix(message, "load config: clickhouse.address is required;") {
		return fmt.Errorf("clickhouse.address is required; set a host:port such as 127.0.0.1:8123")
	}
	if strings.HasPrefix(message, "load config: clickhouse.address ") {
		return fmt.Errorf("load config: clickhouse.address is invalid")
	}
	return fmt.Errorf("load config: configuration is invalid")
}

func verifyClickHouse(ctx context.Context, cfg config.ClickHouseConfig) error {
	probe, err := openClickHouseProbe(clickHouseProbeOptions(cfg))
	if err != nil {
		return redactedConnectionError("open", err, cfg)
	}

	pingErr := probe.Ping(ctx)
	closeErr := probe.Close()
	if pingErr != nil {
		return redactedConnectionError("ping", pingErr, cfg)
	}
	if closeErr != nil {
		return redactedConnectionError("close", closeErr, cfg)
	}
	return nil
}

func clickHouseProbeOptions(cfg config.ClickHouseConfig) *ch.Options {
	options := &ch.Options{
		Protocol: ch.HTTP,
		Addr:     []string{cfg.Address},
		Auth: ch.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
	}
	if cfg.Secure {
		options.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return options
}

func redactedConnectionError(stage string, err error, cfg config.ClickHouseConfig) error {
	return fmt.Errorf("ClickHouse %s failed: %s", stage, redactPasswordForms(err.Error(), cfg.Username, cfg.Password))
}

func redactPasswordForms(message, username, password string) string {
	if password == "" {
		return message
	}
	forms := []string{
		password,
		url.QueryEscape(password),
		url.PathEscape(password),
		url.UserPassword(username, password).String(),
	}
	sort.Slice(forms, func(i, j int) bool { return len(forms[i]) > len(forms[j]) })
	for _, form := range forms {
		message = strings.ReplaceAll(message, form, "[redacted]")
	}
	return message
}
