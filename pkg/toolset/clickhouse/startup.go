package clickhouse

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
)

const (
	startupRoleOptOutMessage = "database service role is not permitted; allow_unsafe_database_role is set, so the startup role check is skipped"
	showCurrentGrantsSQL     = "SHOW GRANTS FINAL"
	showAIFunctionSettingSQL = "SELECT toString(is_obsolete), toString(getSetting('allow_experimental_ai_functions')) FROM system.settings WHERE name = 'allow_experimental_ai_functions'"
	showAIFunctionValueSQL   = "SELECT toString(getSetting('allow_experimental_ai_functions'))"
	maxRoleExpansions        = 8
	unsafeServiceRoleError   = "[clickhouse_unsafe_service_role] database service role is not permitted"
)

// alwaysUnsafePrivilegeNames are administrative privileges rejected at any
// scope. SHOW GRANTS prints canonical names; aliases are included because a
// server may emit either form. SYSTEM children are matched by prefix.
var alwaysUnsafePrivilegeNames = []string{
	"ACCESS MANAGEMENT",
	"CREATE USER",
	"ALTER USER",
	"DROP USER",
	"CREATE TOKEN",
	"CREATE ROLE",
	"ALTER ROLE",
	"DROP ROLE",
	"ROLE ADMIN",
	"CREATE ROW POLICY",
	"ALTER ROW POLICY",
	"DROP ROW POLICY",
	"CREATE POLICY",
	"ALTER POLICY",
	"DROP POLICY",
	"CREATE MASKING POLICY",
	"ALTER MASKING POLICY",
	"DROP MASKING POLICY",
	"CREATE QUOTA",
	"ALTER QUOTA",
	"DROP QUOTA",
	"CREATE SETTINGS PROFILE",
	"ALTER SETTINGS PROFILE",
	"DROP SETTINGS PROFILE",
	"CREATE PROFILE",
	"ALTER PROFILE",
	"DROP PROFILE",
	"SHOW ACCESS",
	"SHOW USERS",
	"SHOW ROLES",
	"SHOW ROW POLICIES",
	"SHOW QUOTAS",
	"SHOW SETTINGS PROFILES",
	"SHOW MASKING POLICIES",
	"SHOW POLICIES",
	"SHOW PROFILES",
	"SHOW CREATE USER",
	"SHOW CREATE ROLE",
	"SHOW CREATE ROW POLICY",
	"SHOW CREATE POLICY",
	"SHOW CREATE QUOTA",
	"SHOW CREATE SETTINGS PROFILE",
	"SHOW CREATE PROFILE",
	"ALLOW SQL SECURITY NONE",
	"CREATE SQL SECURITY NONE",
	"SQL SECURITY NONE",
	"SECURITY NONE",
	"IMPERSONATE",
	"SOURCES",
	"READ",
	"WRITE",
	"AZURE",
	"BIGQUERY",
	"DISK",
	"ELASTICSEARCH",
	"FILE",
	"HDFS",
	"HIVE",
	"JDBC",
	"KAFKA",
	"MONGO",
	"MYSQL",
	"NATS",
	"ODBC",
	"POSTGRES",
	"RABBITMQ",
	"REDIS",
	"REMOTE",
	"S3",
	"SQLITE",
	"URL",
	// Current servers still grant these sources as their own privileges when
	// read/write source grants are disabled. SHOW GRANTS prints ARROW_FLIGHT
	// as "ARROW FLIGHT"; the other spellings are accepted aliases.
	"YTSAURUS",
	"ARROW FLIGHT",
	"ARROW_FLIGHT",
	"ARROWFLIGHT",
	// Global visibility privileges. readonly=1 does not hide other users'
	// connections or SQL HTTP handlers.
	"SHOW CONNECTIONS",
	"SHOW CONNECTION",
	"SHOW HANDLERS",
	"SHOW HANDLER",
	"SHUTDOWN",
	"BACKUP",
	"KILL QUERY",
	"KILL TRANSACTION",
	"MOVE PARTITION BETWEEN SHARDS",
	"NAMED COLLECTION ADMIN",
	"NAMED COLLECTION CONTROL",
	"NAMED COLLECTION",
	"SHOW NAMED COLLECTIONS",
	"SHOW NAMED COLLECTIONS SECRETS",
	"CREATE NAMED COLLECTION",
	"ALTER NAMED COLLECTION",
	"DROP NAMED COLLECTION",
	"SET DEFINER",
	"DISPLAYSECRETSINSHOWANDSELECT",
	"TABLE ENGINE",
	"CLUSTER",
}

// globalDDLPrivilegeNames are schema-changing privileges rejected only when
// granted on *.* . The same privilege on one database is not a service-role
// rejection; tool queries still run with readonly=1.
var globalDDLPrivilegeNames = []string{
	"ALL",
	"ALL PRIVILEGES",
	"CREATE",
	"CREATE DATABASE",
	"CREATE TABLE",
	"CREATE VIEW",
	"CREATE DICTIONARY",
	"CREATE TEMPORARY TABLE",
	"CREATE ARBITRARY TEMPORARY TABLE",
	"CREATE TEMPORARY VIEW",
	"CREATE FUNCTION",
	"CREATE WORKLOAD",
	"CREATE RESOURCE",
	"CREATE HANDLER",
	"ALTER",
	"ALTER DATABASE",
	"ALTER DATABASE SETTINGS",
	"ALTER TABLE",
	"ALTER COLUMN",
	"ALTER ADD COLUMN",
	"ALTER CLEAR COLUMN",
	"ALTER COMMENT COLUMN",
	"ALTER DROP COLUMN",
	"ALTER MATERIALIZE COLUMN",
	"ALTER RENAME COLUMN",
	"ALTER MODIFY COLUMN",
	"ALTER MODIFY COMMENT",
	"ALTER MODIFY DATABASE COMMENT",
	"ALTER ORDER BY",
	"ALTER SAMPLE BY",
	"ALTER INDEX",
	"ALTER ADD INDEX",
	"ALTER DROP INDEX",
	"ALTER MATERIALIZE INDEX",
	"ALTER CLEAR INDEX",
	"ALTER STATISTICS",
	"ALTER ADD STATISTICS",
	"ALTER DROP STATISTICS",
	"ALTER MODIFY STATISTICS",
	"ALTER MATERIALIZE STATISTICS",
	"ALTER PROJECTION",
	"ALTER ADD PROJECTION",
	"ALTER DROP PROJECTION",
	"ALTER MODIFY PROJECTION",
	"ALTER MATERIALIZE PROJECTION",
	"ALTER CLEAR PROJECTION",
	"ALTER CONSTRAINT",
	"ALTER ADD CONSTRAINT",
	"ALTER DROP CONSTRAINT",
	"ALTER MODIFY CONSTRAINT",
	"ALTER TTL",
	"ALTER MATERIALIZE TTL",
	"ALTER REWRITE PARTS",
	"ALTER SETTINGS",
	"ALTER MOVE PARTITION",
	"ALTER FETCH PARTITION",
	"ALTER FREEZE PARTITION",
	"ALTER UNLOCK SNAPSHOT",
	"ALTER EXECUTE",
	"ALTER VIEW",
	"ALTER VIEW MODIFY QUERY",
	"ALTER VIEW REFRESH",
	"ALTER VIEW MODIFY REFRESH",
	"ALTER VIEW MODIFY SQL SECURITY",
	"ALTER HANDLER",
	"ALTER UPDATE",
	"ALTER DELETE",
	"DROP",
	"DROP DATABASE",
	"DROP TABLE",
	"DROP VIEW",
	"DROP DICTIONARY",
	"DROP FUNCTION",
	"DROP WORKLOAD",
	"DROP RESOURCE",
	"DROP HANDLER",
	"TRUNCATE",
	"OPTIMIZE",
	"UNDROP TABLE",
}

var alwaysUnsafePrivileges = privilegeSet(alwaysUnsafePrivilegeNames)
var globalDDLPrivileges = privilegeSet(globalDDLPrivilegeNames)

func privilegeSet(names []string) map[string]struct{} {
	privileges := make(map[string]struct{}, len(names))
	for _, name := range names {
		privileges[name] = struct{}{}
	}
	return privileges
}

type startupQuerier interface {
	queryRaw(context.Context, string, map[string]string) (RowCursor, error)
}

type unavailableClient struct{}

func (unavailableClient) Query(context.Context, string, map[string]string) (RowCursor, error) {
	return nil, errors.New("ClickHouse client is not connected")
}

func newToolset(ctx context.Context, cfg config.ClickHouseConfig, allowUnsafeDatabaseRole bool, open func(config.ClickHouseConfig) (QueryClient, error)) (*Toolset, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Database) == "" {
		cfg.Database = "default"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if open == nil {
		return nil, startupFailure(cfg.Password, errors.New("client is not configured"))
	}
	client, err := open(cfg)
	if err != nil || client == nil {
		if err == nil {
			err = errors.New("client is not configured")
		}
		return nil, startupFailure(cfg.Password, err)
	}
	toolset := newToolsetWithClient(cfg.Address, cfg.Database, cfg.Password, client)
	if err := toolset.verifyStartup(ctx, allowUnsafeDatabaseRole); err != nil {
		_ = toolset.Close()
		return nil, err
	}
	return toolset, nil
}

func (t *Toolset) verifyStartup(ctx context.Context, allowUnsafeDatabaseRole bool) error {
	unsafe, err := t.serviceAccountUnsafe(ctx)
	if err != nil {
		return t.startupFailure(err)
	}
	if !unsafe {
		return nil
	}
	if allowUnsafeDatabaseRole {
		log.Warn().Msg(startupRoleOptOutMessage)
		return nil
	}
	return errors.New(unsafeServiceRoleError)
}

func (t *Toolset) startupFailure(err error) error {
	return startupFailure(t.password, err)
}

func startupFailure(password string, err error) error {
	return fmt.Errorf("[clickhouse_startup_failed] ClickHouse startup check failed: %s", redactSecret(err.Error(), password))
}

func (t *Toolset) serviceAccountUnsafe(ctx context.Context) (bool, error) {
	grants, err := t.readGrantStatements(ctx, showCurrentGrantsSQL)
	if err != nil {
		return false, err
	}
	unsafe, roles, err := classifyGrants(grants)
	if err != nil {
		return false, err
	}
	if unsafe {
		return true, nil
	}

	seen := make(map[string]struct{}, len(roles))
	pending := uniqueNewRoles(seen, roles)
	for round := 0; len(pending) > 0; round++ {
		if round >= maxRoleExpansions {
			return false, errors.New("role grant expansion exceeded the startup limit")
		}
		query, err := showGrantsForRoles(pending)
		if err != nil {
			return false, err
		}
		expanded, err := t.readGrantStatements(ctx, query)
		if err != nil {
			return false, err
		}
		expandedUnsafe, expandedRoles, err := classifyGrants(expanded)
		if err != nil {
			return false, err
		}
		if expandedUnsafe {
			return true, nil
		}
		pending = uniqueNewRoles(seen, expandedRoles)
	}
	return t.aiFunctionsEnabledBySetting(ctx)
}

func (t *Toolset) readGrantStatements(ctx context.Context, query string) ([]string, error) {
	rows, err := t.queryStartup(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	columns := rows.Columns()
	if len(columns) == 0 {
		return nil, errors.New("grant probe returned no columns")
	}
	scanTypes := rows.ScanTypes()
	grants := make([]string, 0)
	for rows.Next() {
		_, destinations := scanDestinations(len(columns), scanTypes)
		if err := rows.Scan(destinations...); err != nil {
			return nil, err
		}
		text, err := grantText(scannedValue(destinations[0]))
		if err != nil {
			return nil, err
		}
		grants = append(grants, text)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return grants, nil
}

func (t *Toolset) queryStartup(ctx context.Context, query string) (RowCursor, error) {
	if raw, ok := t.client.(startupQuerier); ok {
		return raw.queryRaw(ctx, query, nil)
	}
	return t.client.Query(ctx, query, nil)
}

func grantText(value any) (string, error) {
	if value == nil {
		return "", errors.New("grant probe returned an empty grant")
	}
	if bytes, ok := value.([]byte); ok {
		value = string(bytes)
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	text = strings.TrimSuffix(text, ";")
	text = strings.TrimSpace(text)
	if text == "" || text == "<nil>" {
		return "", errors.New("grant probe returned an empty grant")
	}
	return text, nil
}

func showGrantsForRoles(roles []string) (string, error) {
	quoted := make([]string, len(roles))
	for i, role := range roles {
		if !validRoleName(role) {
			return "", errors.New("invalid role name in grant probe")
		}
		quoted[i] = quoteIdent(role)
	}
	return "SHOW GRANTS FOR " + strings.Join(quoted, ", ") + " FINAL", nil
}

func uniqueNewRoles(seen map[string]struct{}, roles []string) []string {
	pending := make([]string, 0)
	for _, role := range roles {
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		pending = append(pending, role)
	}
	sort.Strings(pending)
	return pending
}

func classifyGrants(grants []string) (bool, []string, error) {
	unsafe := false
	var roles []string
	for _, grant := range grants {
		grantUnsafe, grantRoles, err := classifyGrant(grant)
		if err != nil {
			return false, nil, err
		}
		unsafe = unsafe || grantUnsafe
		roles = append(roles, grantRoles...)
	}
	return unsafe, roles, nil
}

func classifyGrant(statement string) (bool, []string, error) {
	statement = strings.TrimSpace(statement)
	statement = strings.TrimSuffix(statement, ";")
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return false, nil, errors.New("empty grant statement")
	}
	if hasKeywordPrefix(statement, "REVOKE") {
		return false, nil, nil
	}
	body, ok := stripGrantPrefix(statement)
	if !ok {
		return false, nil, errors.New("unrecognized grant statement")
	}
	if strings.EqualFold(body, "NONE") {
		return false, nil, nil
	}
	onAt := indexKeywordOutside(body, " ON ")
	toAt := indexKeywordOutside(body, " TO ")
	if onAt >= 0 && toAt > onAt {
		names, err := splitPrivileges(body[:onAt])
		if err != nil {
			return false, nil, err
		}
		scope := strings.TrimSpace(body[onAt+len(" ON ") : toAt])
		if scope == "" {
			return false, nil, errors.New("grant scope is empty")
		}
		return privilegesUnsafe(names, scope), nil, nil
	}
	if toAt >= 0 && onAt < 0 {
		rolesPart := strings.TrimSpace(body[:toAt])
		if strings.EqualFold(rolesPart, "NONE") {
			return false, nil, nil
		}
		roles, err := splitRoleSpecs(rolesPart)
		if err != nil {
			return false, nil, err
		}
		return false, roles, nil
	}
	return false, nil, errors.New("unrecognized grant statement")
}

func stripGrantPrefix(statement string) (string, bool) {
	if !hasKeywordPrefix(statement, "GRANT") {
		return "", false
	}
	body := strings.TrimSpace(statement[len("GRANT"):])
	if body == "" {
		return "", false
	}
	return body, true
}

func hasKeywordPrefix(value, keyword string) bool {
	if len(value) < len(keyword) || !strings.EqualFold(value[:len(keyword)], keyword) {
		return false
	}
	if len(value) == len(keyword) {
		return true
	}
	return value[len(keyword)] == ' ' || value[len(keyword)] == '\t' || value[len(keyword)] == '\n'
}

func privilegesUnsafe(names []string, scope string) bool {
	global := isGlobalScope(scope)
	for _, name := range names {
		if privilegeUnsafe(name, global) || selectsQueryText(name, scope) {
			return true
		}
	}
	return false
}

func privilegeUnsafe(name string, global bool) bool {
	if _, ok := alwaysUnsafePrivileges[name]; ok || name == "SYSTEM" || strings.HasPrefix(name, "SYSTEM ") || aiFunctionPrivilege(name) {
		return true
	}
	if !global {
		return false
	}
	_, ok := globalDDLPrivileges[name]
	return ok
}

// queryTextTables are system tables whose rows hold SQL text or account
// credentials. SELECT on *.* covers every one of them. readonly=1 does not
// hide in-flight queries in system.processes.
var queryTextTables = map[string]struct{}{
	"processes":              {},
	"query_log":              {},
	"query_thread_log":       {},
	"query_views_log":        {},
	"user_query_log":         {},
	"query_cache":            {},
	"asynchronous_inserts":   {},
	"asynchronous_query_log": {},
	"users":                  {},
}

// selectsQueryText reports a SELECT or ALL grant that can read query text or
// account credentials. A partial REVOKE does not remove the global GRANT line
// from SHOW GRANTS FINAL, and it leaves the other tables readable.
func selectsQueryText(name, scope string) bool {
	switch name {
	case "SELECT", "ALL", "ALL PRIVILEGES":
	default:
		return false
	}
	normalized := normalizeScope(scope)
	if normalized == "*.*" {
		return true
	}
	parts := strings.Split(normalized, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return true
	}
	if !strings.EqualFold(parts[0], "system") {
		return false
	}
	if parts[1] == "*" {
		return true
	}
	_, ok := queryTextTables[strings.ToLower(parts[1])]
	return ok
}

// aiFunctionPrivilege reports the AI FUNCTIONS group and one AI function
// privilege. SHOW GRANTS prints the group as "AI FUNCTIONS" and each function
// as a single token such as "aiGenerate". normalizePrivilege uppercases that
// token to "AIGENERATE". These are SELECT functions, so readonly=1 does not
// stop them once a server actually grants the privilege.
func (t *Toolset) aiFunctionsEnabledBySetting(ctx context.Context) (bool, error) {
	enabled, err := t.readAIFunctionSetting(ctx, showAIFunctionSettingSQL)
	if err != nil && obsoleteColumnMissing(err) {
		return t.readAIFunctionValue(ctx)
	}
	return enabled, err
}

func (t *Toolset) readAIFunctionSetting(ctx context.Context, query string) (bool, error) {
	rows, err := t.queryStartup(ctx, query)
	if err != nil {
		if unknownSetting(err) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	if len(rows.Columns()) < 2 {
		return false, errors.New("AI function setting probe returned too few columns")
	}
	_, destinations := scanDestinations(len(rows.Columns()), rows.ScanTypes())
	if err := rows.Scan(destinations...); err != nil {
		return false, err
	}
	obsolete, err := grantText(scannedValue(destinations[0]))
	if err != nil {
		return false, err
	}
	value, err := grantText(scannedValue(destinations[1]))
	if err != nil {
		return false, err
	}
	obsoleteFlag, err := settingFlag(obsolete)
	if err != nil {
		return false, err
	}
	if obsoleteFlag {
		return false, nil
	}
	return settingFlag(value)
}

func (t *Toolset) readAIFunctionValue(ctx context.Context) (bool, error) {
	rows, err := t.queryStartup(ctx, showAIFunctionValueSQL)
	if err != nil {
		if unknownSetting(err) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, errors.New("AI function setting probe returned no rows")
	}
	if len(rows.Columns()) == 0 {
		return false, errors.New("AI function setting probe returned no columns")
	}
	_, destinations := scanDestinations(len(rows.Columns()), rows.ScanTypes())
	if err := rows.Scan(destinations...); err != nil {
		return false, err
	}
	value, err := grantText(scannedValue(destinations[0]))
	if err != nil {
		return false, err
	}
	return settingFlag(value)
}

func settingFlag(text string) (bool, error) {
	switch strings.ToLower(text) {
	case "1", "true":
		return true, nil
	case "0", "false":
		return false, nil
	default:
		return false, fmt.Errorf("unrecognized AI function setting %q", text)
	}
}

func obsoleteColumnMissing(err error) bool {
	text := err.Error()
	if !strings.Contains(text, "is_obsolete") {
		return false
	}
	return strings.Contains(text, "UNKNOWN_IDENTIFIER") ||
		strings.Contains(text, "NO_SUCH_COLUMN") ||
		strings.Contains(text, "Missing columns") ||
		strings.Contains(text, "There is no column")
}

func unknownSetting(err error) bool {
	text := err.Error()
	return strings.Contains(text, "UNKNOWN_SETTING") || strings.Contains(text, "Unknown setting")
}

func aiFunctionPrivilege(name string) bool {
	if name == "AI FUNCTIONS" {
		return true
	}
	if len(name) < 3 || strings.Contains(name, " ") {
		return false
	}
	return strings.HasPrefix(name, "AI")
}

func isGlobalScope(scope string) bool {
	return normalizeScope(scope) == "*.*"
}

func normalizeScope(scope string) string {
	var normalized strings.Builder
	normalized.Grow(len(scope))
	for _, r := range scope {
		if r == '`' || r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		normalized.WriteRune(r)
	}
	return normalized.String()
}

func splitPrivileges(list string) ([]string, error) {
	parts, err := splitCommaOutside(list)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		name := normalizePrivilege(part)
		if name == "" {
			return nil, errors.New("empty privilege name")
		}
		names = append(names, name)
	}
	return names, nil
}

func normalizePrivilege(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.IndexByte(name, '('); i >= 0 {
		name = name[:i]
	}
	return strings.ToUpper(strings.Join(strings.Fields(name), " "))
}

func splitRoleSpecs(list string) ([]string, error) {
	parts, err := splitCommaOutside(list)
	if err != nil {
		return nil, err
	}
	roles := make([]string, 0, len(parts))
	for _, part := range parts {
		name, err := parseIdent(part)
		if err != nil {
			return nil, err
		}
		roles = append(roles, name)
	}
	return roles, nil
}

func splitCommaOutside(list string) ([]string, error) {
	var parts []string
	var current strings.Builder
	inBacktick := false
	inSingle := false
	depth := 0
	for i := 0; i < len(list); i++ {
		ch := list[i]
		switch {
		case inBacktick:
			current.WriteByte(ch)
			if ch != '`' {
				continue
			}
			if i+1 < len(list) && list[i+1] == '`' {
				current.WriteByte(list[i+1])
				i++
				continue
			}
			inBacktick = false
		case inSingle:
			current.WriteByte(ch)
			if ch == '\\' && i+1 < len(list) {
				current.WriteByte(list[i+1])
				i++
				continue
			}
			if ch != '\'' {
				continue
			}
			if i+1 < len(list) && list[i+1] == '\'' {
				current.WriteByte(list[i+1])
				i++
				continue
			}
			inSingle = false
		default:
			switch ch {
			case '`':
				inBacktick = true
				current.WriteByte(ch)
			case '\'':
				inSingle = true
				current.WriteByte(ch)
			case '(':
				depth++
				current.WriteByte(ch)
			case ')':
				if depth == 0 {
					return nil, errors.New("unbalanced grant syntax")
				}
				depth--
				current.WriteByte(ch)
			case ',':
				if depth != 0 {
					current.WriteByte(ch)
					continue
				}
				part := strings.TrimSpace(current.String())
				if part == "" {
					return nil, errors.New("empty grant list item")
				}
				parts = append(parts, part)
				current.Reset()
			default:
				current.WriteByte(ch)
			}
		}
	}
	if inBacktick || inSingle || depth != 0 {
		return nil, errors.New("unbalanced grant syntax")
	}
	part := strings.TrimSpace(current.String())
	if part == "" {
		return nil, errors.New("empty grant list item")
	}
	parts = append(parts, part)
	return parts, nil
}

func indexKeywordOutside(value, keyword string) int {
	if keyword == "" || len(value) < len(keyword) {
		return -1
	}
	upper := strings.ToUpper(value)
	key := strings.ToUpper(keyword)
	inBacktick := false
	inSingle := false
	depth := 0
	for i := 0; i < len(value); i++ {
		ch := value[i]
		switch {
		case inBacktick:
			if ch != '`' {
				continue
			}
			if i+1 < len(value) && value[i+1] == '`' {
				i++
				continue
			}
			inBacktick = false
		case inSingle:
			if ch == '\\' && i+1 < len(value) {
				i++
				continue
			}
			if ch != '\'' {
				continue
			}
			if i+1 < len(value) && value[i+1] == '\'' {
				i++
				continue
			}
			inSingle = false
		default:
			switch ch {
			case '`':
				inBacktick = true
			case '\'':
				inSingle = true
			case '(':
				depth++
			case ')':
				if depth > 0 {
					depth--
				}
			default:
				if depth == 0 && i+len(key) <= len(value) && upper[i:i+len(key)] == key {
					return i
				}
			}
		}
	}
	return -1
}

func parseIdent(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("empty identifier")
	}
	if token[0] == '`' {
		name, rest, err := unquoteIdent(token)
		if err != nil || strings.TrimSpace(rest) != "" {
			return "", errors.New("invalid identifier")
		}
		if !validRoleName(name) {
			return "", errors.New("invalid role name in grant probe")
		}
		return name, nil
	}
	for i := 0; i < len(token); i++ {
		if (i == 0 && !isIdentStart(token[i])) || (i > 0 && !isIdentContinue(token[i])) {
			return "", errors.New("invalid identifier")
		}
	}
	if !validRoleName(token) {
		return "", errors.New("invalid role name in grant probe")
	}
	return token, nil
}

func unquoteIdent(token string) (string, string, error) {
	if token == "" || token[0] != '`' {
		return "", "", errors.New("invalid identifier")
	}
	var name strings.Builder
	for i := 1; i < len(token); i++ {
		if token[i] == '`' {
			if i+1 < len(token) && token[i+1] == '`' {
				name.WriteByte('`')
				i++
				continue
			}
			if name.Len() == 0 {
				return "", "", errors.New("invalid identifier")
			}
			return name.String(), token[i+1:], nil
		}
		if token[i] < 0x20 || token[i] == 0x7f {
			return "", "", errors.New("invalid identifier")
		}
		name.WriteByte(token[i])
	}
	return "", "", errors.New("invalid identifier")
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func validRoleName(name string) bool {
	if name == "" || len(name) > 255 {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] < 0x20 || name[i] == 0x7f || name[i] == ';' || name[i] == '\\' {
			return false
		}
	}
	return true
}

func isIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isIdentContinue(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}
