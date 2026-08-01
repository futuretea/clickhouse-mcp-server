# ClickHouse MCP Server

[中文文档](README.zh.md)

A small, read-only MCP server for exploring ClickHouse from an AI client or the command line. It connects to ClickHouse only over HTTP(S) through [`clickhouse-go/v2`](https://github.com/ClickHouse/clickhouse-go), and exposes Streamable HTTP at `/mcp`.

The MCP service has no authentication. It listens on `127.0.0.1` by default. Keep it on a trusted local network unless you separately design the authentication and network boundary.

## Quick start

1. Copy the example configuration and set the ClickHouse HTTP address.

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

The query tool accepts exactly one statement. Semicolons in strings, quoted identifiers, and SQL comments are allowed; a second statement is rejected before it reaches ClickHouse. Every query is sent with ClickHouse `readonly=1`, so ClickHouse rejects writes and DDL for a user that honors that setting. The connected ClickHouse user should still have only the privileges appropriate for read access.

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

The server never logs the configured password. `secure: true` protects the connection to ClickHouse; it does not add TLS or authentication to the MCP HTTP endpoint.

### Environment-only configuration check

For the smallest local setup, set just the ClickHouse HTTP address and inspect the effective configuration. This command does not start MCP or access the network.

```bash
export MCP_CLICKHOUSE_ADDRESS=127.0.0.1:8123
./bin/clickhouse-mcp-server config check
```

The command prints the 13 configuration fields in a stable order with their source (`flag`, `environment`, `YAML`, or `default`). Passwords are reported only as `set` or `unset`. `sse_base_url` removes query and fragment values and redacts URL userinfo before display.

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

## Troubleshooting

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

The default test suite has no external dependency. The opt-in integration test uses the configured ClickHouse HTTP endpoint to verify database discovery, `SELECT`, and that `readonly=1` rejects temporary DDL. Use an isolated instance: the test deliberately attempts a temporary table creation to prove the read-only setting is enforced.

```bash
docker run -d --rm --name clickhouse-mcp-integration \
  -e CLICKHOUSE_SKIP_USER_SETUP=1 \
  -p 127.0.0.1:8123:8123 \
  clickhouse/clickhouse-server:26.5.6.64

MCP_CLICKHOUSE_ADDRESS=127.0.0.1:8123 make test-integration
docker stop clickhouse-mcp-integration
```

Set `MCP_CLICKHOUSE_DATABASE`, `MCP_CLICKHOUSE_SECURE`, `MCP_CLICKHOUSE_USERNAME`, and `MCP_CLICKHOUSE_PASSWORD` when the target needs a different isolated ClickHouse configuration. The target runs both the read-only toolset integration test and `config check --connect`; it fails when `MCP_CLICKHOUSE_ADDRESS` is absent and never silently skips the integration check.

The repository retains its template assets under `scripts/` and `templates/` for local reference; they are not part of the ClickHouse server runtime.

## License

[MIT](LICENSE)
