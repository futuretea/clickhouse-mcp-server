//go:build integration

package clickhouse

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
)

func TestIntegrationReadOnlyClickHouse(t *testing.T) {
	cfg, err := config.LoadConfig("", viper.New())
	if err != nil {
		t.Fatalf("load integration configuration: %v", err)
	}
	toolset, err := NewToolset(t.Context(), cfg.ClickHouse, cfg.AllowUnsafeDatabaseRole)
	if err != nil {
		t.Fatalf("create ClickHouse toolset: %v", err)
	}
	t.Cleanup(func() { _ = toolset.Close() })
	handlers := handlersByName(toolset.GetTools())

	databases, err := handlers["clickhouse_list_databases"](t.Context(), nil)
	if err != nil {
		t.Fatalf("list databases: %v", err)
	}
	var databaseResult struct {
		Databases []string `json:"databases"`
	}
	if err := json.Unmarshal([]byte(databases), &databaseResult); err != nil {
		t.Fatalf("decode databases: %v", err)
	}
	if len(databaseResult.Databases) == 0 {
		t.Fatal("list databases returned no databases")
	}

	tables, err := handlers["clickhouse_list_tables"](t.Context(), map[string]any{"database": "system"})
	if err != nil {
		t.Fatalf("list system tables: %v", err)
	}
	var tableResult struct {
		Database string `json:"database"`
		Tables   []struct {
			Name string `json:"name"`
		} `json:"tables"`
	}
	if err := json.Unmarshal([]byte(tables), &tableResult); err != nil {
		t.Fatalf("decode system tables: %v", err)
	}
	if tableResult.Database != "system" || len(tableResult.Tables) == 0 {
		t.Fatalf("unexpected system tables result: %s", tables)
	}

	description, err := handlers["clickhouse_describe_table"](t.Context(), map[string]any{"database": "system", "table": "tables"})
	if err != nil {
		t.Fatalf("describe system.tables: %v", err)
	}
	var descriptionResult struct {
		Database string `json:"database"`
		Table    string `json:"table"`
		Columns  []struct {
			Name string `json:"name"`
		} `json:"columns"`
	}
	if err := json.Unmarshal([]byte(description), &descriptionResult); err != nil {
		t.Fatalf("decode system.tables description: %v", err)
	}
	if descriptionResult.Database != "system" || descriptionResult.Table != "tables" || len(descriptionResult.Columns) == 0 {
		t.Fatalf("unexpected system.tables description: %s", description)
	}

	query, err := handlers["clickhouse_query"](t.Context(), map[string]any{"query": "SELECT 1 AS value"})
	if err != nil {
		t.Fatalf("SELECT query: %v", err)
	}
	var queryResult queryResult
	if err := json.Unmarshal([]byte(query), &queryResult); err != nil {
		t.Fatalf("decode SELECT result: %v", err)
	}
	if queryResult.RowCount != 1 || len(queryResult.Columns) != 1 || queryResult.Columns[0] != "value" ||
		len(queryResult.Rows) != 1 || len(queryResult.Rows[0]) != 1 || queryResult.Rows[0][0] != "1" {
		t.Fatalf("unexpected SELECT result: %s", query)
	}

	_, err = handlers["clickhouse_query"](t.Context(), map[string]any{
		"query": "CREATE TEMPORARY TABLE clickhouse_mcp_readonly_test (id UInt8)",
	})
	// CREATE TEMPORARY TABLE is global-only. Granting it on *.* is rejected at
	// startup, so this SELECT/SHOW account is denied the privilege (code 497)
	// rather than reaching the readonly=1 check.
	if err == nil || !temporaryDDLDenied(err.Error()) {
		t.Fatalf("temporary DDL error = %v, want ClickHouse ACCESS_DENIED for CREATE TEMPORARY TABLE", err)
	}
}

func TestIntegrationWildcardProcessGrantRejected(t *testing.T) {
	if os.Getenv("MCP_CLICKHOUSE_WILDCARD_PROBE") != "1" {
		t.Skip("wildcard process grant probe is not enabled")
	}
	cfg, err := config.LoadConfig("", viper.New())
	if err != nil {
		t.Fatalf("load integration configuration: %v", err)
	}
	toolset, err := NewToolset(t.Context(), cfg.ClickHouse, cfg.AllowUnsafeDatabaseRole)
	if toolset != nil {
		t.Fatal("NewToolset returned a toolset for a prefix wildcard on system.processes")
	}
	if err == nil || !strings.Contains(err.Error(), "clickhouse_unsafe_service_role") {
		t.Fatalf("NewToolset error = %v, want clickhouse_unsafe_service_role", err)
	}
}

func TestIntegrationAIFunctionPrivilegeRejected(t *testing.T) {
	if os.Getenv("MCP_CLICKHOUSE_AI_PROBE") != "1" {
		t.Skip("AI FUNCTIONS privilege probe is not enabled")
	}
	cfg, err := config.LoadConfig("", viper.New())
	if err != nil {
		t.Fatalf("load integration configuration: %v", err)
	}
	toolset, err := NewToolset(t.Context(), cfg.ClickHouse, cfg.AllowUnsafeDatabaseRole)
	if toolset != nil {
		t.Fatal("NewToolset returned a toolset for an AI function privilege")
	}
	if err == nil || !strings.Contains(err.Error(), "clickhouse_unsafe_service_role") {
		t.Fatalf("NewToolset error = %v, want clickhouse_unsafe_service_role", err)
	}
}

func temporaryDDLDenied(message string) bool {
	text := strings.ToLower(message)
	return strings.Contains(text, "access_denied") &&
		strings.Contains(text, "code: 497") &&
		strings.Contains(text, "create temporary table")
}
