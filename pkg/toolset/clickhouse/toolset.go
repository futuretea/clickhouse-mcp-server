package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
	"github.com/futuretea/clickhouse-mcp-server/pkg/toolset"
)

const (
	// Domain identifies the ClickHouse tool group.
	Domain = "clickhouse"

	defaultMaxRows = 1000
)

// Toolset exposes read-only ClickHouse discovery and query tools.
type Toolset struct {
	address  string
	database string
	password string
	client   QueryClient
}

// NewToolset connects to ClickHouse and rejects dangerous service accounts
// before any tool can run a query. allowUnsafeDatabaseRole skips only that
// privilege rejection; connection and grant-probe failures still abort.
func NewToolset(ctx context.Context, cfg config.ClickHouseConfig, allowUnsafeDatabaseRole bool) (*Toolset, error) {
	return newToolset(ctx, cfg, allowUnsafeDatabaseRole, openQueryClient)
}

// NewCatalogToolset returns tool definitions without connecting to ClickHouse.
// Listing and describing tools use it so those commands do not open a session
// or run the startup privilege check.
func NewCatalogToolset(cfg config.ClickHouseConfig) *Toolset {
	if strings.TrimSpace(cfg.Database) == "" {
		cfg.Database = "default"
	}
	return newToolsetWithClient(cfg.Address, cfg.Database, cfg.Password, unavailableClient{})
}

func openQueryClient(cfg config.ClickHouseConfig) (QueryClient, error) {
	return newDriverClient(cfg)
}

func newToolsetWithClient(address, database, password string, client QueryClient) *Toolset {
	return &Toolset{address: address, database: database, password: password, client: client}
}

// Close releases the underlying ClickHouse connection pool when present.
func (t *Toolset) Close() error {
	client, ok := t.client.(interface{ Close() error })
	if !ok {
		return nil
	}
	return client.Close()
}

// GetName returns the ClickHouse toolset name.
func (*Toolset) GetName() string { return Domain }

// GetDescription returns a user-facing summary of the ClickHouse tools.
func (*Toolset) GetDescription() string {
	return "Read-only ClickHouse database discovery, table inspection, and SQL query tools"
}

// GetTools returns the tools exposed through MCP.
func (t *Toolset) GetTools() []toolset.ServerTool {
	return []toolset.ServerTool{
		{
			Tool: mcp.NewTool("clickhouse_list_databases",
				mcp.WithDescription("List databases visible to the configured ClickHouse user."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
			),
			Handler: t.listDatabases,
			Domain:  Domain,
		},
		{
			Tool: mcp.NewTool("clickhouse_list_tables",
				mcp.WithDescription("List tables in a database. Uses the configured database when omitted."),
				mcp.WithString("database", mcp.Description("Database to inspect. Defaults to clickhouse.database.")),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
			),
			Handler: t.listTables,
			Domain:  Domain,
		},
		{
			Tool: mcp.NewTool("clickhouse_describe_table",
				mcp.WithDescription("Return column names, types, defaults, and comments for one table."),
				mcp.WithString("table", mcp.Required(), mcp.Description("Table name to inspect.")),
				mcp.WithString("database", mcp.Description("Database containing the table. Defaults to clickhouse.database.")),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
			),
			Handler: t.describeTable,
			Domain:  Domain,
		},
		{
			Tool: mcp.NewTool("clickhouse_query",
				mcp.WithDescription("Run one read-only SQL statement. Results are returned as JSON text and are limited to max_rows."),
				mcp.WithString("query", mcp.Required(), mcp.Description("One read-only ClickHouse SQL statement.")),
				mcp.WithInteger("max_rows", mcp.Min(1), mcp.DefaultNumber(defaultMaxRows), mcp.Description("Maximum rows in the result; defaults to 1000.")),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
			),
			Handler: t.query,
			Domain:  Domain,
		},
	}
}

func (t *Toolset) listDatabases(ctx context.Context, _ map[string]any) (string, error) {
	result, err := t.readRows(ctx, "list databases", "SELECT name FROM system.databases ORDER BY name", nil, -1)
	if err != nil {
		return "", err
	}
	databases := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		databases = append(databases, cellString(row, 0))
	}
	return marshalResult(struct {
		Databases []string `json:"databases"`
	}{Databases: databases})
}

func (t *Toolset) listTables(ctx context.Context, params map[string]any) (string, error) {
	database, err := t.optionalString(params, "database")
	if err != nil {
		return "", err
	}
	result, err := t.readRows(ctx, "list tables", "SELECT name, engine, comment FROM system.tables WHERE database = {database:String} ORDER BY name", map[string]string{"database": database}, -1)
	if err != nil {
		return "", err
	}
	tables := make([]table, 0, len(result.Rows))
	for _, row := range result.Rows {
		tables = append(tables, table{Name: cellString(row, 0), Engine: cellString(row, 1), Comment: cellString(row, 2)})
	}
	return marshalResult(struct {
		Database string  `json:"database"`
		Tables   []table `json:"tables"`
	}{Database: database, Tables: tables})
}

func (t *Toolset) describeTable(ctx context.Context, params map[string]any) (string, error) {
	tableName, err := requiredString(params, "table")
	if err != nil {
		return "", err
	}
	database, err := t.optionalString(params, "database")
	if err != nil {
		return "", err
	}
	result, err := t.readRows(ctx, "describe table", "SELECT name, type, default_kind, default_expression, comment FROM system.columns WHERE database = {database:String} AND table = {table:String} ORDER BY position", map[string]string{"database": database, "table": tableName}, -1)
	if err != nil {
		return "", err
	}
	columns := make([]column, 0, len(result.Rows))
	for _, row := range result.Rows {
		columns = append(columns, column{
			Name:              cellString(row, 0),
			Type:              cellString(row, 1),
			DefaultKind:       cellString(row, 2),
			DefaultExpression: cellString(row, 3),
			Comment:           cellString(row, 4),
		})
	}
	return marshalResult(struct {
		Database string   `json:"database"`
		Table    string   `json:"table"`
		Columns  []column `json:"columns"`
	}{Database: database, Table: tableName, Columns: columns})
}

func (t *Toolset) query(ctx context.Context, params map[string]any) (string, error) {
	statement, err := requiredString(params, "query")
	if err != nil {
		return "", err
	}
	if err := validateSingleStatement(statement); err != nil {
		return "", err
	}
	maxRows, err := parseMaxRows(params["max_rows"])
	if err != nil {
		return "", err
	}
	result, err := t.readRows(ctx, "run query", statement, nil, maxRows)
	if err != nil {
		return "", err
	}
	return marshalResult(result)
}

func (t *Toolset) optionalString(params map[string]any, key string) (string, error) {
	if params == nil || params[key] == nil {
		return t.database, nil
	}
	value, ok := params[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}
	return strings.TrimSpace(value), nil
}

func requiredString(params map[string]any, key string) (string, error) {
	if params == nil {
		return "", fmt.Errorf("%s is required", key)
	}
	value, ok := params[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required and must be a non-empty string", key)
	}
	return strings.TrimSpace(value), nil
}

func parseMaxRows(value any) (int, error) {
	if value == nil {
		return defaultMaxRows, nil
	}
	var number int64
	switch parsed := value.(type) {
	case int:
		number = int64(parsed)
	case int64:
		number = parsed
	case float64:
		if math.Trunc(parsed) != parsed || parsed > math.MaxInt64 || parsed < math.MinInt64 {
			return 0, fmt.Errorf("max_rows must be a positive integer")
		}
		number = int64(parsed)
	default:
		return 0, fmt.Errorf("max_rows must be a positive integer")
	}
	if number < 1 || number > int64(maxInt()) {
		return 0, fmt.Errorf("max_rows must be a positive integer")
	}
	return int(number), nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func marshalResult(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode ClickHouse result: %w", err)
	}
	return string(encoded), nil
}

type table struct {
	Name    string `json:"name"`
	Engine  string `json:"engine"`
	Comment string `json:"comment"`
}

type column struct {
	Name              string `json:"name"`
	Type              string `json:"type"`
	DefaultKind       string `json:"default_kind"`
	DefaultExpression string `json:"default_expression"`
	Comment           string `json:"comment"`
}
