//go:build integration

package clickhouse

import (
	"encoding/json"
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
	toolset, err := NewToolset(cfg.ClickHouse)
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
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "readonly") {
		t.Fatalf("temporary DDL error = %v, want ClickHouse readonly rejection", err)
	}
}
