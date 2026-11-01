# ClickHouse MCP Server

[中文文档](README.zh.md)

A small, read-only MCP server for exploring ClickHouse from an AI client or the command line. It connects to ClickHouse only over HTTP(S) through [`clickhouse-go/v2`](https://github.com/ClickHouse/clickhouse-go), and exposes Streamable HTTP at `/mcp`.

The MCP service has no authentication. It listens on `127.0.0.1` by default. Keep it on a trusted local network unless you separately design the authentication and network boundary.

## Quick start

1. Copy the example configuration and set the ClickHouse HTTP address. Use a read-only user; the built-in `default` user is rejected at startup. See [Service account](#service-account).

   ```bash
   cp config.example.yaml config.yaml
   ```

2. Build and start the HTTP server.

   ```bash
   make build
   ./bin/clickhouse-mcp-server mcp --config config.yaml --port 8080
   ```

3. Confirm that the process is healthy.

   ```bash
   curl -s http://127.0.0.1:8080/healthz
   # healthy
   ```

4. Configure an MCP client with the Streamable HTTP endpoint.

   ```json
   {
     "mcpServers": {
       "clickhouse": {
         "url": "http://127.0.0.1:8080/mcp"
       }
     }
   }
   ```

## Tools

All successful tool calls return JSON in MCP text content.

| Tool | Input | Result |
|------|-------|--------|
| `clickhouse_list_databases` | none | `{"databases":["default"]}` |
| `clickhouse_list_tables` | optional `database` | database name plus table name, engine, and comment |
| `clickhouse_describe_table` | required `table`, optional `database` | column name, type, defaults, and comment |
| `clickhouse_query` | required `query`, optional positive integer `max_rows` | columns, display-string rows, returned `row_count`, and `truncated` |

`clickhouse_query` defaults to 1,000 rows. The server reads one additional row to report `truncated: true`; that extra row is not returned. Each non-null cell is rendered as a string so clients receive a stable JSON shape.

The query tool accepts exactly one statement. Semicolons in strings, quoted identifiers, and SQL comments are allowed; a second statement is rejected before it reaches ClickHouse. Every query is sent with ClickHouse `readonly=1`, so ClickHouse rejects writes and DDL for a user that honors that setting. Startup also rejects accounts that can administer ClickHouse, read external sources, or run global DDL. See [Service account](#service-account).

## Configuration

Configuration priority is **flags > environment variables > YAML file > defaults**. `clickhouse.address` is required and must be a `host:port` value without a scheme or path.

```yaml
port: 8080
listen: 127.0.0.1
log_level: info
clickhouse:
  address: 127.0.0.1:8123
  database: default
  secure: false
  username: ""
  password: ""
allow_unsafe_database_role: false
```

| Group | YAML key | Environment variable | Default | Meaning |
|-------|----------|----------------------|---------|---------|
| ClickHouse connection | `clickhouse.address` | `MCP_CLICKHOUSE_ADDRESS` | none | Required ClickHouse `host:port` address |
| ClickHouse connection | `clickhouse.database` | `MCP_CLICKHOUSE_DATABASE` | `default` | Default database for table tools |
| ClickHouse connection | `clickhouse.secure` | `MCP_CLICKHOUSE_SECURE` | `false` | Use HTTPS instead of HTTP to ClickHouse |
| ClickHouse connection | `clickhouse.username` | `MCP_CLICKHOUSE_USERNAME` | empty | Optional ClickHouse username |
| ClickHouse connection | `clickhouse.password` | `MCP_CLICKHOUSE_PASSWORD` | empty | Optional ClickHouse password |
| Server | `port` | `MCP_PORT` | `0` | HTTP port; `0` starts stdio mode |
| Server | `listen` | `MCP_LISTEN` | `127.0.0.1` | HTTP listen host |
| Server | `sse_base_url` | `MCP_SSE_BASE_URL` | empty | Public base URL for legacy SSE message endpoints |
| Server | `log_level` | `MCP_LOG_LEVEL` | `info` | Server log level |
| Tool filters | `enabled_tools` | `MCP_ENABLED_TOOLS` | empty | Comma-separated tool names to enable |
| Tool filters | `disabled_tools` | `MCP_DISABLED_TOOLS` | empty | Comma-separated tool names to disable |
| Tool filters | `enabled_domains` | `MCP_ENABLED_DOMAINS` | empty | Comma-separated tool domains to enable |
| Tool filters | `disabled_domains` | `MCP_DISABLED_DOMAINS` | empty | Comma-separated tool domains to disable |
| Server | `allow_unsafe_database_role` | `MCP_ALLOW_UNSAFE_DATABASE_ROLE` | `false` | Skip only the startup unsafe-privilege rejection |

The server never logs the configured password. `secure: true` protects the connection to ClickHouse; it does not add TLS or authentication to the MCP HTTP endpoint.

### Service account

After the process connects, and before any tool runs a query, startup reads `SHOW GRANTS FINAL` for the current user. That statement includes privileges inherited through roles. Startup exits with `[clickhouse_unsafe_service_role] database service role is not permitted` when the account has any of:

- `ACCESS MANAGEMENT`, or a privilege in that group, including `CREATE USER`, `ALTER USER`, `DROP USER`, `CREATE ROLE`, `DROP ROLE`, `ROLE ADMIN`, row policies, quotas, settings profiles, and `SHOW ACCESS`
- `SYSTEM`, or any `SYSTEM …` privilege, such as `SYSTEM SHUTDOWN`
- `SOURCES`, `READ`, `WRITE`, or an external source privilege such as `FILE`, `URL`, `REMOTE`, `S3`, `YTSAURUS`, or `ARROW FLIGHT`
- global DDL or global `ALL` / `ALL PRIVILEGES` on `*.*`: `CREATE`, `ALTER`, `DROP`, `TRUNCATE`, `OPTIMIZE`, and their DDL children, including `ALTER HANDLER`
- similar administrative grants: `BACKUP`, `KILL QUERY`, `KILL TRANSACTION`, `MOVE PARTITION BETWEEN SHARDS`, `TABLE ENGINE`, `CLUSTER`, `NAMED COLLECTION ADMIN` (including named-collection secrets), `SET DEFINER`, `ALLOW SQL SECURITY NONE`, `SHOW CONNECTIONS`, and `SHOW HANDLERS`

An account limited to `SELECT` and `SHOW` starts with the default configuration. No opt-out is required. DDL granted on one database, rather than on `*.*`, is not treated as a dangerous service role. Queries are still sent with `readonly=1`.

`allow_unsafe_database_role` defaults to `false`. Set it to `true` in the config file, or export `MCP_ALLOW_UNSAFE_DATABASE_ROLE=true`, only when you cannot provision a least-privilege account. The server logs a warning and skips this privilege rejection. A refused connection, a failed grant probe, and every other startup check still stop startup. `readonly=1` does not constrain `SYSTEM`, `ACCESS MANAGEMENT`, or `SOURCES`: that account can shut the server down, change users, or read server files and remote URLs from a `SELECT`. Enable the option only for a ClickHouse instance you control.

The warning is logged at warn level. HTTP mode shows it at the default `info` log level. Stdio mode keeps logs disabled so MCP output stays intact.

`config check --connect` only pings ClickHouse. It does not run this privilege check. `mcp` and `tools call` do.

Do not use the ClickHouse `default` user. A standard installation grants that user `ALL ON *.*`.

```sql
CREATE USER mcp_reader IDENTIFIED WITH no_password;
GRANT SELECT ON analytics.* TO mcp_reader;
GRANT SELECT ON system.databases TO mcp_reader;
GRANT SELECT ON system.tables TO mcp_reader;
GRANT SELECT ON system.columns TO mcp_reader;
```

The discovery tools read those three system tables. Grant `SELECT` on each database the query tool should read.

### Environment-only configuration check

For the smallest local setup, set just the ClickHouse HTTP address and inspect the effective configuration. This command does not start MCP or access the network.

```bash
export MCP_CLICKHOUSE_ADDRESS=127.0.0.1:8123
./bin/clickhouse-mcp-server config check
```

The command prints the 14 configuration fields in a stable order with their source (`flag`, `environment`, `YAML`, or `default`). Passwords are reported only as `set` or `unset`. `sse_base_url` removes query and fragment values and redacts URL userinfo before display.

Use `--connect` only when you intentionally want one ClickHouse HTTP(S) `Ping`; it opens, pings, and closes a short-lived connection without starting the MCP server.

```bash
./bin/clickhouse-mcp-server config check --connect
```

## CLI

The CLI uses the same configuration and is useful for checking connectivity without an MCP client.

```bash
./bin/clickhouse-mcp-server tools list --config config.yaml
./bin/clickhouse-mcp-server tools describe clickhouse_query --config config.yaml --json
./bin/clickhouse-mcp-server tools call clickhouse_list_databases --config config.yaml
./bin/clickhouse-mcp-server tools call clickhouse_query --config config.yaml \
  --params '{"query":"SELECT version()"}'
```

## Transports

`mcp` starts stdio when `port` is `0`, or an HTTP process when it is non-zero. HTTP mode exposes:

| Path | Purpose |
|------|---------|
| `/mcp` | Primary Streamable HTTP MCP endpoint |
| `/healthz` | Health check (`GET` and `HEAD`) |
| `/sse` | Legacy SSE MCP endpoint |
| `/message` | SSE message endpoint |

## Docker

```bash
make docker
docker run --rm -p 8080:8080 \
  -v "$PWD/config.yaml:/config.yaml:ro" \
  clickhouse-mcp-server:dev --config /config.yaml --port 8080
```

Do not publish the unauthenticated HTTP port directly to the internet.

## Release artifacts

Only stable `vX.Y.Z` Git tags trigger releases. A validated stable release updates the npm, Docker, and GitHub Release `latest` pointers; use a versioned reference when you need a fixed version.

```bash
npx -y @futuretea/clickhouse-mcp-server
npx -y @futuretea/clickhouse-mcp-server@<version>
docker run --rm -i ghcr.io/futuretea/clickhouse-mcp-server:latest
docker run --rm -i ghcr.io/futuretea/clickhouse-mcp-server:v<version>
```

Both commands launch the `mcp` command. Arguments after the package or image are forwarded to it. Maintainers need a GitHub Actions `NPM_TOKEN` with publish permission for `@futuretea`; after the first release, set the GHCR package visibility to public.

## Troubleshooting

- `[clickhouse_unsafe_service_role]`: the configured user can administer ClickHouse, read external sources, or run global DDL. Create a `SELECT`-only user, or set `allow_unsafe_database_role: true` only on an instance you control.
- `clickhouse.address is required`: add a `host:port` value such as `127.0.0.1:8123` to the configuration or `MCP_CLICKHOUSE_ADDRESS`.
- Connection failure: check the address, `secure` value, ClickHouse HTTP port, and network reachability.
- Query rejection: verify the statement is a single read-only query and that the configured ClickHouse user has access to the database.
- `truncated: true`: lower the result size in SQL or increase `max_rows` deliberately.

## Development

```bash
make format
make lint
make test
make ci
```

### Optional ClickHouse integration test

The default test suite has no external dependency. The opt-in integration test starts an isolated local ClickHouse Docker container to verify database discovery, parameterized table inspection, `SELECT`, and that `CREATE TEMPORARY TABLE` is denied for the `SELECT`/`SHOW` account. ClickHouse returns `ACCESS_DENIED` (code 497) because that privilege can only be granted on `*.*`, which startup rejects. It also builds the public server binary, starts it in HTTP mode on loopback, checks `/healthz`, and calls `tools/list` and `tools/call` through `/mcp`.

```bash
make test-integration
```

The target requires Docker and `curl`, creates a random loopback port, waits for ClickHouse health, and removes its container on exit. Set `CLICKHOUSE_INTEGRATION_IMAGE` to test another isolated image. It creates ClickHouse user `mcp_reader` with `SELECT` and `SHOW` on `*.*` and runs the tests as that user, so the startup privilege check passes without `allow_unsafe_database_role`. It runs the full MCP HTTP E2E test, the read-only toolset integration test, and `config check --connect`. It is local-only: neither `make test` nor the GitHub Actions CI workflow runs it.

The repository retains its template assets under `scripts/` and `templates/` for local reference; they are not part of the ClickHouse server runtime.

## License

[MIT](LICENSE)
