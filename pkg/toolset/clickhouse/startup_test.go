package clickhouse

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
)

func TestNewToolsetRejectsDangerousPrivileges(t *testing.T) {
	output := captureStartupLog(t)
	privileges := append([]string{}, alwaysUnsafePrivilegeNames...)
	privileges = append(privileges, "SYSTEM", "SYSTEM SHUTDOWN", "SYSTEM MERGES")
	for _, privilege := range privileges {
		t.Run(privilege, func(t *testing.T) {
			output.Reset()
			client := newStartupClient([][]string{{
				fmt.Sprintf("GRANT %s ON *.* TO `sentinel-account`", privilege),
			}})
			toolset, err := openStartup(t, client, false)
			if toolset != nil {
				t.Fatal("NewToolset returned a toolset for a dangerous account")
			}
			assertUnsafeServiceRole(t, err)
			assertStartupQueries(t, client, []string{showCurrentGrantsSQL})
			if client.closed != 1 {
				t.Fatalf("startup closed the connection count = %d, want 1", client.closed)
			}
			assertNoStartupOptOut(t, output.String())
		})
	}

	t.Run("global DDL", func(t *testing.T) {
		for _, privilege := range globalDDLPrivilegeNames {
			t.Run(privilege, func(t *testing.T) {
				output.Reset()
				client := newStartupClient([][]string{{
					fmt.Sprintf("GRANT %s ON *.* TO reader", privilege),
				}})
				toolset, err := openStartup(t, client, false)
				if toolset != nil {
					t.Fatal("NewToolset returned a toolset for global DDL")
				}
				assertUnsafeServiceRole(t, err)
				assertNoStartupOptOut(t, output.String())
			})
		}
	})

	t.Run("case and clause variants", func(t *testing.T) {
		grants := []string{
			"grant Access Management on *.* to reader",
			"GRANT SYSTEM, SOURCES ON *.* TO reader",
			"GRANT SELECT, FILE ON *.* TO reader WITH GRANT OPTION",
			"GRANT ALL PRIVILEGES ON *.* TO default WITH GRANT OPTION",
			"GRANT READ ON S3 TO reader",
			"GRANT READ ON S3('s3://bucket/.*') TO reader",
			"GRANT READ, WRITE ON URL('https://example.com') TO reader",
			"GRANT CREATE USER ON analytics.* TO reader",
			"GRANT SYSTEM MERGES ON analytics.events TO reader",
			"GRANT DROP TABLE ON `*`.`*` TO reader",
			"GRANT TABLE ENGINE ON * TO reader",
		}
		for _, grant := range grants {
			t.Run(grant, func(t *testing.T) {
				output.Reset()
				client := newStartupClient([][]string{{grant}})
				toolset, err := openStartup(t, client, false)
				if toolset != nil {
					t.Fatal("NewToolset returned a toolset for a dangerous grant")
				}
				assertUnsafeServiceRole(t, err)
				assertNoStartupOptOut(t, output.String())
			})
		}
	})

	t.Run("ai function privileges", func(t *testing.T) {
		// Unit fixtures for grant text. ClickHouse 26.5.6.64, 26.5.7.64, and
		// 26.10.2.103 reject GRANT AI FUNCTIONS as a syntax error.
		// aiNotInCatalogYet is the family rule, not a current catalog name.
		grants := []string{
			"GRANT AI FUNCTIONS ON *.* TO reader",
			"GRANT aiGenerate ON *.* TO reader",
			"GRANT aiEmbed ON analytics.* TO reader",
			"GRANT aiClassify, aiExtract, aiFilter, aiRedact, aiSimilarity, aiTranslate ON *.* TO reader",
			"GRANT aiNotInCatalogYet ON *.* TO reader",
		}
		for _, grant := range grants {
			t.Run(grant, func(t *testing.T) {
				output.Reset()
				client := newStartupClient([][]string{{grant}})
				toolset, err := openStartup(t, client, false)
				if toolset != nil {
					t.Fatal("NewToolset returned a toolset for an AI function privilege")
				}
				assertUnsafeServiceRole(t, err)
				assertNoStartupOptOut(t, output.String())
			})
		}
	})

	t.Run("query text grants", func(t *testing.T) {
		grants := []string{
			"GRANT SELECT, SHOW ON *.* TO reader",
			"GRANT SELECT ON system.* TO reader",
			"GRANT SELECT ON system.processes TO reader",
			"GRANT SELECT ON system.query_log TO reader",
			"GRANT SELECT ON system.query_thread_log TO reader",
			"GRANT SELECT ON system.query_views_log TO reader",
			"GRANT SELECT ON system.user_query_log TO reader",
			"GRANT SELECT ON system.query_cache TO reader",
			"GRANT SELECT ON system.asynchronous_inserts TO reader",
			"GRANT SELECT ON system.asynchronous_query_log TO reader",
			"GRANT SELECT ON system.users TO reader",
			"GRANT ALL ON system.processes TO reader",
			"GRANT SELECT ON *.* TO reader",
			"GRANT SELECT ON system.proc* TO reader",
			"GRANT SELECT ON system.user* TO reader",
			"GRANT ALL ON system.proc* TO reader",
			"GRANT SELECT ON sys*.* TO reader",
			"GRANT SELECT ON s*.* TO reader",
			"GRANT SELECT(query, user) ON system.processes TO reader",
		}
		for _, grant := range grants {
			t.Run(grant, func(t *testing.T) {
				output.Reset()
				client := newStartupClient([][]string{{grant}})
				toolset, err := openStartup(t, client, false)
				if toolset != nil {
					t.Fatal("NewToolset returned a toolset for a query-text grant")
				}
				assertUnsafeServiceRole(t, err)
				assertNoStartupOptOut(t, output.String())
			})
		}
		t.Run("partial revoke leaves the global grant", func(t *testing.T) {
			output.Reset()
			client := newStartupClient([][]string{{
				"GRANT SHOW, SELECT ON *.* TO reader",
				"REVOKE SELECT ON system.processes FROM reader",
			}})
			toolset, err := openStartup(t, client, false)
			if toolset != nil {
				t.Fatal("NewToolset returned a toolset after a partial revoke of system.processes")
			}
			assertUnsafeServiceRole(t, err)
			assertNoStartupOptOut(t, output.String())
		})
	})

	t.Run("realistic default user grants", func(t *testing.T) {
		output.Reset()
		client := newStartupClient([][]string{{
			"GRANT CHECK, SHOW, SELECT, INSERT, ALTER, CREATE, DROP, UNDROP TABLE, TRUNCATE, OPTIMIZE, BACKUP, KILL QUERY, KILL TRANSACTION, MOVE PARTITION BETWEEN SHARDS, SYSTEM, dictGet, displaySecretsInShowAndSelect, INTROSPECTION, SOURCES, CLUSTER ON *.* TO default",
			"GRANT TABLE ENGINE ON * TO default",
			"GRANT SET DEFINER ON * TO default",
			"GRANT ALTER NAMED COLLECTION, CREATE NAMED COLLECTION, DROP NAMED COLLECTION, SHOW NAMED COLLECTIONS, NAMED COLLECTION ON * TO default",
		}})
		toolset, err := openStartup(t, client, false)
		if toolset != nil {
			t.Fatal("NewToolset returned a toolset for the default user grant set")
		}
		assertUnsafeServiceRole(t, err)
		assertStartupQueries(t, client, []string{showCurrentGrantsSQL})
		assertNoStartupOptOut(t, output.String())
	})

	t.Run("dangerous privilege inherited through a role", func(t *testing.T) {
		client := newStartupClient([][]string{
			{"GRANT SELECT ON analytics.* TO reader", "GRANT `admin` TO `reader`"},
			{"GRANT FILE ON *.* TO `admin`"},
		})
		toolset, err := openStartup(t, client, false)
		if toolset != nil {
			t.Fatal("NewToolset returned a toolset for a role with FILE")
		}
		assertUnsafeServiceRole(t, err)
		assertStartupQueries(t, client, []string{
			showCurrentGrantsSQL,
			"SHOW GRANTS FOR `admin` FINAL",
		})
	})

	t.Run("dangerous privilege inherited through a nested role", func(t *testing.T) {
		client := newStartupClient([][]string{
			{"GRANT `admin` TO `reader`"},
			{"GRANT `nested` TO `admin`"},
			{"GRANT SYSTEM ON *.* TO `nested`"},
		})
		toolset, err := openStartup(t, client, false)
		if toolset != nil {
			t.Fatal("NewToolset returned a toolset for a nested role with SYSTEM")
		}
		assertUnsafeServiceRole(t, err)
		assertStartupQueries(t, client, []string{
			showCurrentGrantsSQL,
			"SHOW GRANTS FOR `admin` FINAL",
			"SHOW GRANTS FOR `nested` FINAL",
		})
	})
}

func TestNewToolsetAllowsUnsafePrivilegesWhenOptedIn(t *testing.T) {
	output := captureStartupLog(t)
	client := newStartupClient([][]string{{
		"GRANT ACCESS MANAGEMENT, SYSTEM, SOURCES, FILE, AI FUNCTIONS ON *.* TO `sentinel-account`",
		"GRANT aiGenerate ON *.* TO `sentinel-account`",
		"GRANT CREATE TABLE, DROP, ALTER, TRUNCATE ON *.* TO `sentinel-account`",
	}})
	toolset, err := openStartup(t, client, true)
	if err != nil {
		t.Fatalf("NewToolset: %v", err)
	}
	if toolset == nil {
		t.Fatal("NewToolset toolset = nil, want started toolset")
	}
	t.Cleanup(func() { _ = toolset.Close() })
	assertStartupQueries(t, client, []string{showCurrentGrantsSQL})
	if client.closed != 0 {
		t.Fatalf("startup closed the connection count = %d, want 0", client.closed)
	}
	text := output.String()
	for _, want := range []string{`"level":"warn"`, startupRoleOptOutMessage} {
		if !strings.Contains(text, want) {
			t.Fatalf("log = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "dsn-sentinel-password") || strings.Contains(text, "sentinel-account") {
		t.Fatalf("opt-out log leaked account details: %s", text)
	}
}

func TestNewToolsetStartsSafeReadOnlyAccountWithoutOptOut(t *testing.T) {
	output := captureStartupLog(t)
	client := newStartupClient([][]string{{
		"GRANT SELECT, SHOW ON analytics.* TO reader",
		"GRANT SELECT(id, name) ON analytics.events TO reader",
		"GRANT SELECT ON system.databases TO reader",
		"GRANT SELECT ON system.tables TO reader",
		"GRANT SELECT ON system.columns TO reader",
		"GRANT SHOW DATABASES, SHOW TABLES, SHOW COLUMNS ON *.* TO reader",
		"GRANT dictGet ON analytics.* TO reader",
		"GRANT CHECK, SHOW, SELECT, dictGet, INTROSPECTION ON analytics.* TO reader",
		"GRANT INTROSPECTION, addressToLine ON *.* TO reader",
		"GRANT INSERT ON analytics.* TO reader",
		"GRANT NONE ON *.* TO reader",
	}})
	toolset, err := openStartup(t, client, false)
	if err != nil {
		t.Fatalf("NewToolset: %v", err)
	}
	if toolset == nil {
		t.Fatal("NewToolset toolset = nil, want started toolset")
	}
	assertStartupQueries(t, client, []string{showCurrentGrantsSQL, showAIFunctionSettingSQL})
	assertNoStartupOptOut(t, output.String())
	if client.closed != 0 {
		t.Fatalf("startup closed the connection count = %d, want 0", client.closed)
	}
	if err := toolset.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if client.closed != 1 {
		t.Fatalf("Close closed the connection count = %d, want 1", client.closed)
	}
}

func TestNewToolsetAllowsDatabaseDDLAndEmptyGrants(t *testing.T) {
	output := captureStartupLog(t)
	for _, privilege := range globalDDLPrivilegeNames {
		t.Run(privilege, func(t *testing.T) {
			output.Reset()
			client := newStartupClient([][]string{{
				fmt.Sprintf("GRANT %s ON analytics.* TO writer", privilege),
				"GRANT SELECT ON analytics.* TO writer",
			}})
			toolset, err := openStartup(t, client, false)
			if err != nil {
				t.Fatalf("NewToolset: %v", err)
			}
			if toolset == nil {
				t.Fatal("NewToolset toolset = nil, want started toolset")
			}
			_ = toolset.Close()
			assertNoStartupOptOut(t, output.String())
		})
	}

	t.Run("no grants", func(t *testing.T) {
		output.Reset()
		client := newStartupClient([][]string{{}})
		toolset, err := openStartup(t, client, false)
		if err != nil {
			t.Fatalf("NewToolset: %v", err)
		}
		_ = toolset.Close()
		assertNoStartupOptOut(t, output.String())
	})

	t.Run("none privilege", func(t *testing.T) {
		client := newStartupClient([][]string{{"GRANT NONE TO reader"}})
		toolset, err := openStartup(t, client, false)
		if err != nil {
			t.Fatalf("NewToolset: %v", err)
		}
		_ = toolset.Close()
	})

	t.Run("safe role", func(t *testing.T) {
		client := newStartupClient([][]string{
			{"GRANT reader_role TO reader"},
			{"GRANT SELECT ON analytics.* TO reader_role"},
		})
		toolset, err := openStartup(t, client, false)
		if err != nil {
			t.Fatalf("NewToolset: %v", err)
		}
		_ = toolset.Close()
		assertStartupQueries(t, client, []string{
			showCurrentGrantsSQL,
			"SHOW GRANTS FOR `reader_role` FINAL",
			showAIFunctionSettingSQL,
		})
	})

	t.Run("revoke is not a grant", func(t *testing.T) {
		client := newStartupClient([][]string{{
			"REVOKE SYSTEM ON *.* FROM reader",
			"GRANT SELECT ON analytics.* TO reader",
		}})
		toolset, err := openStartup(t, client, false)
		if err != nil {
			t.Fatalf("NewToolset: %v", err)
		}
		_ = toolset.Close()
	})
}

func TestNewToolsetOptOutDoesNotSkipOtherStartupFailures(t *testing.T) {
	output := captureStartupLog(t)

	t.Run("connection failure", func(t *testing.T) {
		output.Reset()
		client := newStartupClient(nil)
		client.err = errors.New("dial tcp 127.0.0.1:8123: connect: connection refused dsn-sentinel-password")
		toolset, err := openStartup(t, client, true)
		if toolset != nil {
			t.Fatal("NewToolset returned a toolset after the connection check failed")
		}
		assertStartupFailure(t, err)
		if len(client.calls) != 1 || client.calls[0].query != showCurrentGrantsSQL {
			t.Fatalf("queries = %#v, want one grant probe", client.calls)
		}
		assertNoStartupOptOut(t, output.String())
		if client.closed != 1 {
			t.Fatalf("startup closed the connection count = %d, want 1", client.closed)
		}
	})

	t.Run("grant probe failure", func(t *testing.T) {
		output.Reset()
		client := newStartupClient([][]string{{"not a grant"}})
		toolset, err := openStartup(t, client, true)
		if toolset != nil {
			t.Fatal("NewToolset returned a toolset after the grant probe failed")
		}
		assertStartupFailure(t, err)
		assertNoStartupOptOut(t, output.String())
		if client.closed != 1 {
			t.Fatalf("startup closed the connection count = %d, want 1", client.closed)
		}
	})

	t.Run("open failure", func(t *testing.T) {
		output.Reset()
		_, err := newToolset(t.Context(), startupConfig(), true, func(config.ClickHouseConfig) (QueryClient, error) {
			return nil, errors.New("open failed dsn-sentinel-password")
		})
		assertStartupFailure(t, err)
		assertNoStartupOptOut(t, output.String())
	})

	t.Run("invalid address does not connect", func(t *testing.T) {
		called := false
		_, err := newToolset(t.Context(), config.ClickHouseConfig{}, true, func(config.ClickHouseConfig) (QueryClient, error) {
			called = true
			return nil, nil
		})
		if called {
			t.Fatal("invalid configuration opened a client")
		}
		if err == nil || !strings.Contains(err.Error(), "clickhouse.address is required") {
			t.Fatalf("NewToolset error = %v", err)
		}
		if strings.Contains(err.Error(), "clickhouse_startup_failed") || strings.Contains(output.String(), startupRoleOptOutMessage) {
			t.Fatalf("validation error = %v log = %s", err, output.String())
		}
	})

	t.Run("role name is not interpolated", func(t *testing.T) {
		client := newStartupClient([][]string{{"GRANT `bad;name` TO reader"}})
		toolset, err := openStartup(t, client, true)
		if toolset != nil {
			t.Fatal("NewToolset returned a toolset for an invalid role name")
		}
		assertStartupFailure(t, err)
		assertStartupQueries(t, client, []string{showCurrentGrantsSQL})
		assertNoStartupOptOut(t, output.String())
	})

	t.Run("role expansion limit", func(t *testing.T) {
		sets := make([][]string, 0, maxRoleExpansions+1)
		sets = append(sets, []string{"GRANT r0 TO reader"})
		for i := 0; i < maxRoleExpansions; i++ {
			sets = append(sets, []string{fmt.Sprintf("GRANT r%d TO r%d", i+1, i)})
		}
		client := newStartupClient(sets)
		toolset, err := openStartup(t, client, true)
		if toolset != nil {
			t.Fatal("NewToolset returned a toolset after role expansion exceeded the limit")
		}
		assertStartupFailure(t, err)
		if len(client.calls) != maxRoleExpansions+1 {
			t.Fatalf("queries = %d, want %d", len(client.calls), maxRoleExpansions+1)
		}
		assertNoStartupOptOut(t, output.String())
	})
}

func TestNewToolsetUsesRawGrantQueryWhenAvailable(t *testing.T) {
	responses := grantResponses([][]string{{
		"GRANT SELECT ON analytics.* TO reader",
	}})
	responses = append(responses, fakeResponse{columns: []string{"is_obsolete", "value"}, rows: [][]any{{"0", "false"}}})
	client := &rawAwareClient{fakeClient: &fakeClient{responses: responses}}
	toolset, err := newToolset(t.Context(), startupConfig(), false, func(config.ClickHouseConfig) (QueryClient, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("NewToolset: %v", err)
	}
	_ = toolset.Close()
	if len(client.rawQueries) != 2 || client.rawQueries[0] != showCurrentGrantsSQL || client.rawQueries[1] != showAIFunctionSettingSQL {
		t.Fatalf("raw queries = %#v, want grant probe then AI setting", client.rawQueries)
	}
}

func TestNewToolsetRejectsEnabledAIFunctionSetting(t *testing.T) {
	output := captureStartupLog(t)
	responses := grantResponses([][]string{{
		"GRANT SELECT ON analytics.* TO reader",
		"GRANT SELECT ON system.databases TO reader",
	}})
	responses = append(responses, fakeResponse{columns: []string{"is_obsolete", "value"}, rows: [][]any{{"0", "true"}}})
	client := newStartupClient(nil)
	client.responses = responses
	toolset, err := openStartup(t, client, false)
	if toolset != nil {
		t.Fatal("NewToolset returned a toolset with AI functions enabled")
	}
	assertUnsafeServiceRole(t, err)
	assertStartupQueries(t, client, []string{showCurrentGrantsSQL, showAIFunctionSettingSQL})
	assertNoStartupOptOut(t, output.String())
}

func TestNewToolsetOptOutDoesNotSkipAISettingProbeFailure(t *testing.T) {
	output := captureStartupLog(t)
	responses := grantResponses([][]string{{"GRANT SELECT ON analytics.* TO reader"}})
	responses = append(responses, fakeResponse{err: errors.New("setting probe failed dsn-sentinel-password")})
	client := newStartupClient(nil)
	client.responses = responses
	toolset, err := openStartup(t, client, true)
	if toolset != nil {
		t.Fatal("NewToolset returned a toolset after the AI setting probe failed")
	}
	assertStartupFailure(t, err)
	assertNoStartupOptOut(t, output.String())
}

func TestNewToolsetAllowsObsoleteAIFunctionSetting(t *testing.T) {
	responses := grantResponses([][]string{{
		"GRANT SELECT ON analytics.* TO reader",
		"GRANT SELECT ON system.databases TO reader",
		"GRANT SELECT ON system.tables TO reader",
		"GRANT SELECT ON system.columns TO reader",
	}})
	responses = append(responses, fakeResponse{columns: []string{"is_obsolete", "value"}, rows: [][]any{{"1", "true"}}})
	client := newStartupClient(nil)
	client.responses = responses
	toolset, err := openStartup(t, client, false)
	if err != nil {
		t.Fatalf("NewToolset: %v", err)
	}
	if toolset == nil {
		t.Fatal("NewToolset toolset = nil, want started toolset")
	}
	_ = toolset.Close()
}

func TestNewToolsetRejectsLiveAIFunctionSettingValue(t *testing.T) {
	output := captureStartupLog(t)
	responses := grantResponses([][]string{{
		"GRANT SELECT ON analytics.* TO reader",
		"GRANT SELECT ON system.databases TO reader",
	}})
	responses = append(responses, fakeResponse{columns: []string{"is_obsolete", "value"}, rows: [][]any{{"0", "true"}}})
	client := newStartupClient(nil)
	client.responses = responses
	toolset, err := openStartup(t, client, false)
	if toolset != nil {
		t.Fatal("NewToolset returned a toolset while the live AI setting is enabled")
	}
	assertUnsafeServiceRole(t, err)
	assertNoStartupOptOut(t, output.String())
}

func TestNewToolsetAllowsReaderWhenAISettingIsUnknown(t *testing.T) {
	responses := grantResponses([][]string{{"GRANT SELECT ON analytics.* TO reader"}})
	responses = append(responses, fakeResponse{err: errors.New("Code: 115. DB::Exception: Unknown setting allow_experimental_ai_functions. (UNKNOWN_SETTING)")})
	client := newStartupClient(nil)
	client.responses = responses
	toolset, err := openStartup(t, client, false)
	if err != nil {
		t.Fatalf("NewToolset: %v", err)
	}
	_ = toolset.Close()
}

func TestNewCatalogToolsetDoesNotConnect(t *testing.T) {
	toolset := NewCatalogToolset(config.ClickHouseConfig{Address: "127.0.0.1:8123"})
	if toolset.database != "default" {
		t.Fatalf("database = %q, want default", toolset.database)
	}
	if len(toolset.GetTools()) != 4 {
		t.Fatalf("tools = %d, want 4", len(toolset.GetTools()))
	}
}

func openStartup(t *testing.T, client *startupClient, allowUnsafe bool) (*Toolset, error) {
	t.Helper()
	return newToolset(t.Context(), startupConfig(), allowUnsafe, func(config.ClickHouseConfig) (QueryClient, error) {
		return client, nil
	})
}

func startupConfig() config.ClickHouseConfig {
	return config.ClickHouseConfig{
		Address:  "127.0.0.1:8123",
		Database: "default",
		Username: "sentinel-account",
		Password: "dsn-sentinel-password",
	}
}

func assertUnsafeServiceRole(t *testing.T, err error) {
	t.Helper()
	if err == nil || err.Error() != unsafeServiceRoleError {
		t.Fatalf("error = %v, want %s", err, unsafeServiceRoleError)
	}
	if strings.Contains(err.Error(), "dsn-sentinel-password") || strings.Contains(err.Error(), "sentinel-account") {
		t.Fatalf("error leaked account details: %v", err)
	}
}

func assertStartupFailure(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.HasPrefix(err.Error(), "[clickhouse_startup_failed] ") {
		t.Fatalf("error = %v, want clickhouse_startup_failed", err)
	}
	if strings.Contains(err.Error(), unsafeServiceRoleError) {
		t.Fatalf("probe failure reported the privilege rejection: %v", err)
	}
	if strings.Contains(err.Error(), "dsn-sentinel-password") {
		t.Fatalf("error leaked the password: %v", err)
	}
}

func assertStartupQueries(t *testing.T, client *startupClient, want []string) {
	t.Helper()
	got := make([]string, len(client.calls))
	for i, call := range client.calls {
		got[i] = call.query
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("startup queries = %#v, want %#v", got, want)
	}
}

func assertNoStartupOptOut(t *testing.T, text string) {
	t.Helper()
	if strings.Contains(text, startupRoleOptOutMessage) || strings.Contains(text, "dsn-sentinel-password") || strings.Contains(text, "sentinel-account") {
		t.Fatalf("startup log = %q, want no opt-out warning or account details", text)
	}
}

func captureStartupLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previousLogger := log.Logger
	previousLevel := zerolog.GlobalLevel()
	log.Logger = zerolog.New(&output)
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	t.Cleanup(func() {
		log.Logger = previousLogger
		zerolog.SetGlobalLevel(previousLevel)
	})
	return &output
}

func newStartupClient(grantSets [][]string) *startupClient {
	responses := grantResponses(grantSets)
	responses = append(responses, fakeResponse{columns: []string{"is_obsolete", "value"}, rows: [][]any{{"0", "false"}}})
	return &startupClient{fakeClient: &fakeClient{responses: responses}}
}

func grantResponses(grantSets [][]string) []fakeResponse {
	responses := make([]fakeResponse, len(grantSets))
	for i, grants := range grantSets {
		rows := make([][]any, len(grants))
		for j, grant := range grants {
			rows[j] = []any{grant}
		}
		responses[i] = fakeResponse{columns: []string{"grants"}, rows: rows}
	}
	return responses
}

type startupClient struct {
	*fakeClient
	closed int
}

func (c *startupClient) Close() error {
	c.closed++
	return nil
}

type rawAwareClient struct {
	*fakeClient
	rawQueries []string
}

func (c *rawAwareClient) queryRaw(ctx context.Context, query string, parameters map[string]string) (RowCursor, error) {
	c.rawQueries = append(c.rawQueries, query)
	return c.Query(ctx, query, parameters)
}
