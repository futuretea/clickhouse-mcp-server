package clickhouse

import (
	"context"
	"crypto/tls"
	"reflect"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
)

// QueryClient is the minimal ClickHouse behavior used by the toolset.
type QueryClient interface {
	Query(context.Context, string, map[string]string) (RowCursor, error)
}

// RowCursor reads one ClickHouse result set.
type RowCursor interface {
	Columns() []string
	ScanTypes() []reflect.Type
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}

type driverClient struct {
	connection ch.Conn
}

func newDriverClient(cfg config.ClickHouseConfig) (*driverClient, error) {
	connection, err := ch.Open(clickHouseOptions(cfg))
	if err != nil {
		return nil, err
	}
	return &driverClient{connection: connection}, nil
}

func clickHouseOptions(cfg config.ClickHouseConfig) *ch.Options {
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

func (c *driverClient) Query(ctx context.Context, query string, parameters map[string]string) (RowCursor, error) {
	queryContext := ch.Context(ctx,
		ch.WithSettings(ch.Settings{"readonly": 1}),
		ch.WithParameters(ch.Parameters(parameters)),
	)
	rows, err := c.connection.Query(queryContext, query)
	if err != nil {
		return nil, err
	}
	return &driverCursor{rows: rows}, nil
}

func (c *driverClient) Close() error {
	return c.connection.Close()
}

type driverCursor struct {
	rows chdriver.Rows
}

func (c *driverCursor) Columns() []string { return c.rows.Columns() }

func (c *driverCursor) ScanTypes() []reflect.Type {
	columnTypes := c.rows.ColumnTypes()
	types := make([]reflect.Type, len(columnTypes))
	for i, columnType := range columnTypes {
		types[i] = columnType.ScanType()
	}
	return types
}

func (c *driverCursor) Next() bool             { return c.rows.Next() }
func (c *driverCursor) Scan(dest ...any) error { return c.rows.Scan(dest...) }
func (c *driverCursor) Err() error             { return c.rows.Err() }
func (c *driverCursor) Close() error           { return c.rows.Close() }
