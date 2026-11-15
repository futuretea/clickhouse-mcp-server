package clickhouse

import (
	"fmt"
	"math/big"
	"net"
	"reflect"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/chcol"
	"github.com/google/uuid"
	"github.com/paulmach/orb"
	"github.com/shopspring/decimal"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/masking"
)

const (
	maxPresentDepth = 32
	maxPresentNodes = 10000
)

type presenter struct {
	policy masking.Policy
	budget *masking.Budget
	nodes  int
}

func (t *Toolset) beginResult() *presenter {
	p := &presenter{policy: t.policy}
	if t.policy.Enabled() {
		p.budget = &masking.Budget{}
	}
	return p
}

func (p *presenter) row(columns []string, values []any) ([]any, error) {
	if p == nil || !p.policy.Enabled() {
		return displayRow(values), nil
	}
	out := make([]any, len(values))
	for i, value := range values {
		name := ""
		if i < len(columns) {
			name = columns[i]
		}
		cell, changed, err := p.present(name, value, 0)
		if err != nil {
			return nil, err
		}
		if !changed {
			cell = legacyCell(value)
		}
		out[i] = cell
	}
	return out, nil
}

func (p *presenter) present(name string, value any, depth int) (any, bool, error) {
	if err := p.noteNode(depth); err != nil {
		return nil, false, err
	}
	value = unwrapPointers(value)
	if variant, ok := value.(chcol.Variant); ok {
		if variant.Nil() {
			return nil, false, nil
		}
		return p.present(name, variant.Any(), depth+1)
	}
	if value == nil {
		return nil, false, nil
	}
	if name != "" && p.policy.MatchName(name) {
		return selectedWhole(value, p.policy.Placeholder()), true, nil
	}
	switch typed := value.(type) {
	case string:
		masked, err := p.policy.MaskText(typed, p.budget)
		if err != nil {
			return nil, false, err
		}
		return masked, masked != typed, nil
	case []byte:
		masked, err := p.policy.MaskBytes(typed, p.budget)
		if err != nil {
			return nil, false, err
		}
		text := string(masked)
		return text, text != string(typed), nil
	case chcol.JSON:
		return p.walkMap(reflect.ValueOf(typed.NestedMap()), depth)
	}
	if rendered, ok := knownDisplay(value); ok {
		return p.scalarText(value, rendered)
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Map:
		return p.walkMap(rv, depth)
	case reflect.Slice, reflect.Array:
		return p.walkSequence(rv, depth)
	default:
		return nil, false, masking.ErrFailed
	}
}

func (p *presenter) scalarText(value any, rendered any) (any, bool, error) {
	text, ok := rendered.(string)
	if !ok || !matchableScalar(value) {
		return nil, false, nil
	}
	hit, err := p.policy.MatchScalar(text, p.budget)
	if err != nil {
		return nil, false, err
	}
	if hit {
		return nil, true, nil
	}
	return nil, false, nil
}

func matchableScalar(value any) bool {
	switch value.(type) {
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64, decimal.Decimal, big.Int,
		time.Time, time.Duration, net.IP, uuid.UUID,
		orb.Point, orb.Ring, orb.LineString, orb.Polygon, orb.MultiLineString, orb.MultiPolygon:
		return true
	default:
		return false
	}
}

func (p *presenter) noteNode(depth int) error {
	p.nodes++
	if p.nodes > maxPresentNodes || depth > maxPresentDepth {
		return masking.ErrFailed
	}
	return nil
}

func (p *presenter) walkMap(rv reflect.Value, depth int) (any, bool, error) {
	if !rv.IsValid() || rv.IsNil() {
		return map[string]any{}, false, nil
	}
	out := make(map[string]any, rv.Len())
	changed := false
	for _, key := range rv.MapKeys() {
		original, err := keyText(key.Interface())
		if err != nil {
			return nil, false, err
		}
		item := rv.MapIndex(key).Interface()
		var keyOut string
		var presented any
		if p.policy.MatchName(original) {
			keyOut = p.policy.Placeholder()
			presented = selectedWhole(item, p.policy.Placeholder())
			changed = true
		} else {
			keyOut, err = p.policy.MaskText(original, p.budget)
			if err != nil {
				return nil, false, err
			}
			if keyOut != original {
				changed = true
			}
			var childChanged bool
			presented, childChanged, err = p.present("", item, depth+1)
			if err != nil {
				return nil, false, err
			}
			if !childChanged {
				presented = legacyCell(item)
			}
			changed = changed || childChanged
		}
		if _, exists := out[keyOut]; exists {
			return nil, false, masking.ErrFailed
		}
		out[keyOut] = presented
	}
	return out, changed, nil
}

func (p *presenter) walkSequence(rv reflect.Value, depth int) (any, bool, error) {
	if rv.Kind() == reflect.Slice && rv.Type().Name() != "" {
		return nil, false, masking.ErrFailed
	}
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return []any{}, false, nil
	}
	out := make([]any, rv.Len())
	changed := false
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i).Interface()
		presented, childChanged, err := p.present("", item, depth+1)
		if err != nil {
			return nil, false, err
		}
		if !childChanged {
			presented = legacyCell(item)
		}
		changed = changed || childChanged
		out[i] = presented
	}
	return out, changed, nil
}

func legacyCell(value any) any {
	if value == nil {
		return nil
	}
	if bytes, ok := value.([]byte); ok {
		return string(bytes)
	}
	return fmt.Sprint(value)
}

func keyText(value any) (string, error) {
	value = unwrapPointers(value)
	if text, ok := value.(string); ok {
		return text, nil
	}
	rendered, ok := knownDisplay(value)
	if !ok {
		return "", masking.ErrFailed
	}
	text, ok := rendered.(string)
	if !ok {
		return "", masking.ErrFailed
	}
	return text, nil
}

func selectedWhole(value any, placeholder string) any {
	value = unwrapPointers(value)
	if variant, ok := value.(chcol.Variant); ok {
		if variant.Nil() {
			return nil
		}
		value = unwrapPointers(variant.Any())
	}
	if value == nil {
		return nil
	}
	switch value.(type) {
	case string, []byte:
		return placeholder
	default:
		return nil
	}
}

func unwrapPointers(value any) any {
	if value == nil {
		return nil
	}
	rv := reflect.ValueOf(value)
	for rv.IsValid() && (rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface) {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return nil
	}
	return rv.Interface()
}

func knownDisplay(value any) (any, bool) {
	if value == nil {
		return nil, true
	}
	switch typed := value.(type) {
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64:
		return fmt.Sprint(typed), true
	case time.Time, time.Duration:
		return fmt.Sprint(typed), true
	case net.IP:
		return fmt.Sprint(typed), true
	case uuid.UUID:
		return fmt.Sprint(typed), true
	case decimal.Decimal:
		return fmt.Sprint(typed), true
	case big.Int:
		return typed.String(), true
	case orb.Point, orb.Ring, orb.LineString, orb.Polygon, orb.MultiLineString, orb.MultiPolygon:
		return fmt.Sprint(typed), true
	default:
		return nil, false
	}
}

func (p *presenter) maskIdentifier(value string) (string, error) {
	if p == nil || !p.policy.Enabled() {
		return value, nil
	}
	if p.policy.MatchName(value) {
		return p.policy.Placeholder(), nil
	}
	return p.policy.MaskText(value, p.budget)
}

func (p *presenter) maskIdentifiers(values []string) ([]string, error) {
	if p == nil || !p.policy.Enabled() {
		return values, nil
	}
	out := make([]string, len(values))
	seen := make(map[string]struct{}, len(values))
	for i, value := range values {
		masked, err := p.maskIdentifier(value)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[masked]; ok {
			return nil, masking.ErrFailed
		}
		seen[masked] = struct{}{}
		out[i] = masked
	}
	return out, nil
}
