package clickhouse

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strings"
)

type queryResult struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	RowCount  int      `json:"row_count"`
	Truncated bool     `json:"truncated"`
}

func (t *Toolset) readRows(ctx context.Context, operation, query string, parameters map[string]string, maxRows int) (queryResult, error) {
	rows, err := t.client.Query(ctx, query, parameters)
	if err != nil {
		return queryResult{}, t.queryError(operation, err)
	}
	defer func() { _ = rows.Close() }()

	result := queryResult{Columns: rows.Columns(), Rows: make([][]any, 0)}
	scanTypes := rows.ScanTypes()
	for rows.Next() {
		if maxRows >= 0 && len(result.Rows) == maxRows {
			result.Truncated = true
			break
		}
		values, destinations := scanDestinations(len(result.Columns), scanTypes)
		if err := rows.Scan(destinations...); err != nil {
			return queryResult{}, t.queryError(operation, err)
		}
		for i, destination := range destinations {
			values[i] = scannedValue(destination)
		}
		result.Rows = append(result.Rows, displayRow(values))
	}
	if err := rows.Err(); err != nil {
		return queryResult{}, t.queryError(operation, err)
	}
	result.RowCount = len(result.Rows)
	return result, nil
}

func scanDestinations(columnCount int, scanTypes []reflect.Type) ([]any, []any) {
	values := make([]any, columnCount)
	destinations := make([]any, columnCount)
	for i := range values {
		if i >= len(scanTypes) || scanTypes[i] == nil {
			destinations[i] = &values[i]
			continue
		}
		destinations[i] = reflect.New(scanTypes[i]).Interface()
	}
	return values, destinations
}

func scannedValue(destination any) any {
	value := reflect.ValueOf(destination)
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	return value.Interface()
}

func (t *Toolset) queryError(operation string, err error) error {
	message := err.Error()
	if t.password != "" {
		message = strings.ReplaceAll(message, t.password, "[redacted]")
		message = strings.ReplaceAll(message, url.QueryEscape(t.password), "[redacted]")
	}
	return fmt.Errorf("ClickHouse %s failed at %s (database %q): %s; check clickhouse.address, clickhouse.secure, clickhouse.database, and the SQL", operation, t.address, t.database, message)
}

func displayRow(values []any) []any {
	row := make([]any, len(values))
	for i, value := range values {
		if value == nil {
			continue
		}
		if bytes, ok := value.([]byte); ok {
			row[i] = string(bytes)
			continue
		}
		row[i] = fmt.Sprint(value)
	}
	return row
}

func cellString(row []any, index int) string {
	if index >= len(row) || row[index] == nil {
		return ""
	}
	return fmt.Sprint(row[index])
}
