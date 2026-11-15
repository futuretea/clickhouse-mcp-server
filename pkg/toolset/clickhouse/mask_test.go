package clickhouse

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/chcol"
	"github.com/shopspring/decimal"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/masking"
)

const (
	sentinelValue = "synthetic-token-value"
	sentinelName  = "synthetic_field"
	liveToken     = "sk_live_4242"
	largeExactInt = "18446744073709551615"
)

func TestDisabledQueryPreservesSentinelBytes(t *testing.T) {
	encoded := mustQuery(t, masking.Disabled(), []string{"label", "nested"}, [][]any{{
		sentinelValue,
		map[string]any{"keep": sentinelValue},
	}})
	if !strings.Contains(encoded, sentinelValue) || !strings.Contains(encoded, "map[") {
		t.Fatalf("disabled result = %s", encoded)
	}
}

func TestEnabledQueryRemovesSentinelAndKeepsUnmatchedLegacyCells(t *testing.T) {
	policy := mustPolicy(t, `{"enabled":true,"names":["synthetic_field"],"values":["synthetic-token-value"]}`)
	plain := [][]any{{42, nil, "ordinary", true, []byte("plain")}}
	columns := []string{"n", "missing", "label", "flag", "raw"}
	if got, want := mustQuery(t, policy, columns, plain), mustQuery(t, masking.Disabled(), columns, plain); got != want {
		t.Fatalf("unmatched enabled = %s\ndisabled = %s", got, want)
	}

	encoded := mustQuery(t, policy, []string{"label", "payload"}, [][]any{{
		"prefix " + sentinelValue + " suffix",
		map[string]any{
			"keep":          "ordinary",
			sentinelName:    sentinelValue,
			"token_session": "visible",
			"nested":        []any{sentinelValue, "keep"},
			"count":         7,
		},
	}})
	if strings.Contains(encoded, sentinelValue) || strings.Contains(encoded, sentinelName) {
		t.Fatalf("enabled result still contains configured data: %s", encoded)
	}
	var result queryResult
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatalf("decode enabled result: %v", err)
	}
	if result.Rows[0][0] != "prefix [masked] suffix" {
		t.Fatalf("string cell = %#v", result.Rows[0][0])
	}
	payload, ok := result.Rows[0][1].(map[string]any)
	if !ok {
		t.Fatalf("matched nested cell = %#v, want an object", result.Rows[0][1])
	}
	if payload["keep"] != "ordinary" || payload["count"] != "7" || payload["[masked]"] != "[masked]" {
		t.Fatalf("nested object = %#v", payload)
	}
	nested, ok := payload["nested"].([]any)
	if !ok || len(nested) != 2 || nested[0] != "[masked]" || nested[1] != "keep" {
		t.Fatalf("nested array = %#v", payload["nested"])
	}
}

func TestUnmatchedNestedMapStaysLegacyDisplay(t *testing.T) {
	policy := mustPolicy(t, `{"enabled":true,"values":["synthetic-token-value"]}`)
	encoded := mustQuery(t, policy, []string{"payload"}, [][]any{{
		map[string]any{"keep": "ordinary", "n": 7},
	}})
	var result queryResult
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	text, ok := result.Rows[0][0].(string)
	if !ok || !strings.Contains(text, "map[") || !strings.Contains(text, "ordinary") || !strings.Contains(text, "7") {
		t.Fatalf("unmatched nested cell = %#v", result.Rows[0][0])
	}
}

func TestValueMatchNullsNumbersBoolAndDecimal(t *testing.T) {
	policy := mustPolicy(t, `{"enabled":true,"values":["18446744073709551615","1.23","411111","true"]}`)
	amount := decimal.RequireFromString("1.2300")
	encoded := mustQuery(t, policy, []string{"wide", "amount", "flag", "other", "nested"}, [][]any{{
		uint64(18446744073709551615),
		amount,
		true,
		false,
		map[string]any{"wide": uint64(18446744073709551615), "keep": int64(3)},
	}})
	if strings.Contains(encoded, largeExactInt) || strings.Contains(encoded, "1.23") || strings.Contains(encoded, "411111") {
		t.Fatalf("numeric value remained: %s", encoded)
	}
	var result queryResult
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	row := result.Rows[0]
	if row[0] != nil || row[1] != nil || row[2] != nil || row[3] != "false" {
		t.Fatalf("scalar row = %#v", row)
	}
	nested, ok := row[4].(map[string]any)
	if !ok || nested["wide"] != nil || nested["keep"] != "3" {
		t.Fatalf("nested numbers = %#v", row[4])
	}
}

func TestNameCollisionAndUnknownTypeFailClosed(t *testing.T) {
	policy := mustPolicy(t, `{"enabled":true,"names":["alpha","beta"]}`)
	client := &fakeClient{responses: []fakeResponse{{
		columns: []string{"alpha", "beta"},
		rows:    [][]any{{sentinelValue, "ordinary"}},
	}}}
	_, err := handlersByName(newToolsetWithClient("127.0.0.1:8123", "default", "", client, policy).GetTools())["clickhouse_query"](
		t.Context(), map[string]any{"query": "SELECT 1"},
	)
	if err == nil || err.Error() != maskingFailedMessage || strings.Contains(err.Error(), sentinelValue) || strings.Contains(err.Error(), "alpha") {
		t.Fatalf("collision error = %v", err)
	}

	type hidden struct{ Token string }
	client = &fakeClient{responses: []fakeResponse{{
		columns: []string{"value"},
		rows:    [][]any{{hidden{Token: sentinelValue}}},
	}}}
	_, err = handlersByName(newToolsetWithClient("127.0.0.1:8123", "default", "", client, policy).GetTools())["clickhouse_query"](
		t.Context(), map[string]any{"query": "SELECT 1"},
	)
	if err == nil || err.Error() != maskingFailedMessage || strings.Contains(err.Error(), sentinelValue) {
		t.Fatalf("unknown type error = %v", err)
	}
}

func TestQueryKeepsCommandAndMaxRows(t *testing.T) {
	policy := mustPolicy(t, `{"enabled":true,"values":["synthetic-token-value"]}`)
	statement := "SELECT '" + sentinelValue + "' AS label"
	client := &fakeClient{responses: []fakeResponse{{
		columns: []string{"label"},
		rows:    [][]any{{sentinelValue}, {"ordinary"}, {sentinelValue + "-extra"}},
	}}}
	encoded, err := handlersByName(newToolsetWithClient("127.0.0.1:8123", "default", "", client, policy).GetTools())["clickhouse_query"](
		t.Context(), map[string]any{"query": statement, "max_rows": 2},
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if strings.Contains(encoded, sentinelValue) || !strings.Contains(encoded, `"truncated":true`) {
		t.Fatalf("result = %s", encoded)
	}
	if client.calls[0].query != statement {
		t.Fatalf("recorded query = %q", client.calls[0].query)
	}
}

func TestDiscoveryMasksMetadata(t *testing.T) {
	policy := mustPolicy(t, `{"enabled":true,"values":["synthetic-token-value"],"names":["token_hidden"]}`)
	client := &fakeClient{responses: []fakeResponse{
		{columns: []string{"name", "engine", "comment"}, rows: [][]any{{"events", "MergeTree", "see " + sentinelValue}}},
		{columns: []string{"name", "type", "default_kind", "default_expression", "comment"}, rows: [][]any{{
			"token_hidden", "Nullable(String)", "DEFAULT", sentinelValue, "ordinary",
		}}},
	}}
	handlers := handlersByName(newToolsetWithClient("127.0.0.1:8123", "default", "", client, policy).GetTools())
	tables, err := handlers["clickhouse_list_tables"](t.Context(), map[string]any{"database": "analytics"})
	if err != nil || strings.Contains(tables, sentinelValue) || !strings.Contains(tables, "[masked]") {
		t.Fatalf("tables = %s, %v", tables, err)
	}
	if client.calls[0].parameters["database"] != "analytics" {
		t.Fatalf("table parameters = %#v", client.calls[0].parameters)
	}
	described, err := handlers["clickhouse_describe_table"](t.Context(), map[string]any{"database": "analytics", "table": "events"})
	if err != nil || strings.Contains(described, sentinelValue) || strings.Contains(described, "token_hidden") {
		t.Fatalf("describe = %s, %v", described, err)
	}
}

func TestActiveQueryErrorOmitsDriverText(t *testing.T) {
	policy := mustPolicy(t, `{"enabled":true,"values":["synthetic-token-value"]}`)
	client := &fakeClient{err: errors.New("driver said " + sentinelValue + " password=secret")}
	_, err := handlersByName(newToolsetWithClient("127.0.0.1:8123", "analytics", "secret", client, policy).GetTools())["clickhouse_query"](
		t.Context(), map[string]any{"query": "SELECT 1"},
	)
	if err == nil || err.Error() != queryFailedMessage || strings.Contains(err.Error(), sentinelValue) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %v", err)
	}
}

func TestActiveStartupAndSettingErrorsOmitValues(t *testing.T) {
	policy := mustPolicy(t, `{"enabled":true,"values":["synthetic-token-value"]}`)
	err := startupFailure("secret", errors.New("setting "+sentinelValue), policy)
	if err == nil || err.Error() != startupFailedMessage || strings.Contains(err.Error(), sentinelValue) {
		t.Fatalf("startup error = %v", err)
	}
	toolset := newToolsetWithClient("127.0.0.1:8123", "default", "", &fakeClient{}, policy)
	_, err = toolset.settingFlag(sentinelValue)
	if err == nil || strings.Contains(err.Error(), sentinelValue) {
		t.Fatalf("setting error = %v", err)
	}
}

func TestPresentDepthAndJSONVariant(t *testing.T) {
	policy := mustPolicy(t, `{"enabled":true,"names":["synthetic_field"],"values":["synthetic-token-value"]}`)
	var deep any = sentinelValue
	for i := 0; i < 40; i++ {
		deep = []any{deep}
	}
	err := mustQueryError(t, policy, []string{"value"}, [][]any{{deep}})
	if err == nil || err.Error() != maskingFailedMessage || strings.Contains(err.Error(), sentinelValue) {
		t.Fatalf("deep error = %v", err)
	}

	object := chcol.NewJSON()
	object.SetValueAtPath(sentinelName, sentinelValue)
	object.SetValueAtPath("keep", "ordinary")
	encoded := mustQuery(t, policy, []string{"doc", "item"}, [][]any{{
		*object,
		chcol.NewVariant(sentinelValue),
	}})
	if strings.Contains(encoded, sentinelValue) || strings.Contains(encoded, sentinelName) || !strings.Contains(encoded, "ordinary") {
		t.Fatalf("composite result = %s", encoded)
	}
}

func TestNamePatternAnchorRejectsBreakoutAndEmitsWholeNames(t *testing.T) {
	const secret = sentinelValue
	raw := `{"enabled":true,"name_patterns":["a)|(b"],"values":["` + secret + `"]}`
	_, err := masking.ParseJSON(raw)
	if err == nil || err.Error() != masking.InvalidConfigMessage || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "a)|(b") {
		t.Fatalf("breakout error = %v", err)
	}

	policy := mustPolicy(t, `{"enabled":true,"name_patterns":["(?:a)|(b)"]}`)
	encoded := mustQuery(t, policy, []string{"aSECRET", "b", "zzb", "ab"}, [][]any{{
		"ordinary", secret, "tail", "ab-value",
	}})
	if strings.Contains(encoded, secret) || strings.Contains(encoded, `"b"`) {
		t.Fatalf("whole-name result = %s", encoded)
	}
	var result queryResult
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !reflect.DeepEqual(result.Columns, []string{"aSECRET", "[masked]", "zzb", "ab"}) {
		t.Fatalf("columns = %#v", result.Columns)
	}
	if !reflect.DeepEqual(result.Rows[0], []any{"ordinary", "[masked]", "tail", "ab-value"}) {
		t.Fatalf("row = %#v", result.Rows[0])
	}
}

func TestQuerySharesOneBudgetAcrossCells(t *testing.T) {
	policy := mustPolicy(t, `{"enabled":true,"values":["synthetic-token-value"]}`)
	chunk := strings.Repeat("b", 600000)
	one := mustQuery(t, policy, []string{"label"}, [][]any{{chunk}})
	if strings.Contains(one, sentinelValue) || !strings.Contains(one, chunk) {
		t.Fatalf("single cell length = %d", len(one))
	}
	_, err := mustQueryErrorOrResult(t, policy, []string{"left", "right"}, [][]any{{chunk, chunk}})
	if err == nil || err.Error() != maskingFailedMessage {
		t.Fatalf("shared budget error = %v", err)
	}
}

func TestInvalidUTF8BytesStillApplyValuePatterns(t *testing.T) {
	policy := mustPolicy(t, `{"enabled":true,"value_patterns":["sk_live_[0-9]+"]}`)
	cell := append([]byte{0xff}, []byte(liveToken)...)
	encoded, err := mustQueryErrorOrResult(t, policy, []string{"raw"}, [][]any{{cell}})
	if err != nil {
		if strings.Contains(err.Error(), liveToken) {
			t.Fatalf("error echoed the token: %v", err)
		}
		return
	}
	if strings.Contains(encoded, liveToken) {
		t.Fatal("invalid UTF-8 cell still contained the configured token")
	}
}

func TestInvalidUTF8LiteralExpansionIsNotReturned(t *testing.T) {
	placeholder := strings.Repeat("x", 64)
	policy := mustPolicy(t, `{"enabled":true,"placeholder":"`+placeholder+`","values":["a"]}`)
	cell := append([]byte{0xff}, bytesRepeat('a', 20000)...)
	encoded, err := mustQueryErrorOrResult(t, policy, []string{"raw"}, [][]any{{cell}})
	if err == nil {
		if len(encoded) > 1000 {
			t.Fatalf("expanded output length = %d", len(encoded))
		}
		t.Fatal("expanded invalid UTF-8 cell was returned")
	}
	if err.Error() != maskingFailedMessage || strings.Contains(err.Error(), placeholder) {
		t.Fatalf("expansion error = %v", err)
	}
}

func TestZeroWidthAlternativeDoesNotKeepToken(t *testing.T) {
	raw := `{"enabled":true,"value_patterns":["\\b|sk_live_4242"]}`
	policy, err := masking.ParseJSON(raw)
	if err != nil {
		if strings.Contains(err.Error(), liveToken) || err.Error() != masking.InvalidConfigMessage {
			t.Fatalf("parse error = %v", err)
		}
		return
	}
	got, err := policy.MaskText(liveToken, nil)
	if err != nil {
		if strings.Contains(err.Error(), liveToken) {
			t.Fatalf("mask error echoed the token: %v", err)
		}
		return
	}
	if strings.Contains(got, liveToken) {
		t.Fatalf("accepted pattern left the token in place: %q", got)
	}
}

func TestNestedJSONNumbersUseExactText(t *testing.T) {
	policy := mustPolicy(t, `{"enabled":true,"values":["18446744073709551615","1.2300","true"]}`)
	document := `{"n":18446744073709551615,"d":1.2300,"ok":true,"s":"ordinary"}`
	encoded := mustQuery(t, policy, []string{"doc"}, [][]any{{document}})
	if strings.Contains(encoded, largeExactInt) || strings.Contains(encoded, "1.2300") || strings.Contains(encoded, `"ok":true`) {
		t.Fatalf("nested JSON still contains configured numbers: %s", encoded)
	}
	if !strings.Contains(encoded, "ordinary") {
		t.Fatalf("nested JSON dropped an unmatched string: %s", encoded)
	}
}

func mustPolicy(t *testing.T, raw string) masking.Policy {
	t.Helper()
	policy, err := masking.ParseJSON(raw)
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	return policy
}

func mustQuery(t *testing.T, policy masking.Policy, columns []string, rows [][]any) string {
	t.Helper()
	encoded, err := mustQueryErrorOrResult(t, policy, columns, rows)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	return encoded
}

func mustQueryError(t *testing.T, policy masking.Policy, columns []string, rows [][]any) error {
	t.Helper()
	_, err := mustQueryErrorOrResult(t, policy, columns, rows)
	return err
}

func mustQueryErrorOrResult(t *testing.T, policy masking.Policy, columns []string, rows [][]any) (string, error) {
	t.Helper()
	client := &fakeClient{responses: []fakeResponse{{columns: columns, rows: rows}}}
	return handlersByName(newToolsetWithClient("127.0.0.1:8123", "default", "", client, policy).GetTools())["clickhouse_query"](
		context.Background(), map[string]any{"query": "SELECT 1"},
	)
}

func bytesRepeat(value byte, count int) []byte {
	out := make([]byte, count)
	for i := range out {
		out[i] = value
	}
	return out
}
