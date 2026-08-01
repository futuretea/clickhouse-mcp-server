package clickhouse

import (
	"fmt"
	"strings"
)

func validateSingleStatement(query string) error {
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("query is required")
	}

	const (
		normal = iota
		singleQuote
		doubleQuote
		backtick
		lineComment
		blockComment
	)

	state := normal
	hasStatement := false
	terminated := false
	for i := 0; i < len(query); i++ {
		current := query[i]
		switch state {
		case normal:
			if isWhitespace(current) {
				continue
			}
			if current == '-' && i+1 < len(query) && query[i+1] == '-' {
				state = lineComment
				i++
				continue
			}
			if current == '#' {
				state = lineComment
				continue
			}
			if current == '/' && i+1 < len(query) && query[i+1] == '*' {
				state = blockComment
				i++
				continue
			}
			if current == ';' {
				if !hasStatement || terminated {
					return fmt.Errorf("query must contain exactly one SQL statement")
				}
				terminated = true
				continue
			}
			if terminated {
				return fmt.Errorf("query must contain exactly one SQL statement")
			}
			hasStatement = true
			switch current {
			case '\'':
				state = singleQuote
			case '"':
				state = doubleQuote
			case '`':
				state = backtick
			}
		case lineComment:
			if current == '\n' || current == '\r' {
				state = normal
			}
		case blockComment:
			if current == '*' && i+1 < len(query) && query[i+1] == '/' {
				state = normal
				i++
			}
		case singleQuote, doubleQuote, backtick:
			if current == '\\' && i+1 < len(query) {
				i++
				continue
			}
			if (state == singleQuote && current == '\'') ||
				(state == doubleQuote && current == '"') ||
				(state == backtick && current == '`') {
				if i+1 < len(query) && query[i+1] == current {
					i++
					continue
				}
				state = normal
			}
		}
	}

	if state == blockComment {
		return fmt.Errorf("query has an unterminated block comment")
	}
	if state != normal && state != lineComment {
		return fmt.Errorf("query has an unterminated quoted value")
	}
	if !hasStatement {
		return fmt.Errorf("query is required")
	}
	return nil
}

func isWhitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}
