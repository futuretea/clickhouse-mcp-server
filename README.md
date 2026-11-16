# ClickHouse MCP Server

[中文文档](README.zh.md)

This server lets an MCP client list ClickHouse databases and tables and run one read-only SQL statement. It connects with [`clickhouse-go` v2](https://github.com/ClickHouse/clickhouse-go) over HTTP or HTTPS. It does not open the ClickHouse native TCP port.

`mcp` uses stdio when `port` is `0`. That is the default. A non-zero port starts HTTP. The HTTP routes do not check credentials. The default listen address is `127.0.0.1`. Keep that port on a trusted network, or put your own authentication and network boundary in front of it.

The current npm `latest` release and the current GHCR `latest` image are `0.2.1`.

## Install the current release

### npm

Install a pinned release:

```bash
npx -y @futuretea/clickhouse-mcp-server@0.2.1 --help
```

The process prints help for `mcp` and exits 0. The package [on npm](https://www.npmjs.com/package/@futuretea/clickhouse-mcp-server) is a wrapper. Its `bin` entry runs `clickhouse-mcp-server mcp` and appends the arguments you passed. Those arguments are flags for `mcp`, such as `--port`. They are not subcommands.

```bash
npx -y @futuretea/clickhouse-mcp-server@0.2.1 version
```

This does not print the version. The wrapper still starts `mcp`. With no ClickHouse address, the process exits 1 and prints:

```text
load config: clickhouse.address is required; set a host:port such as 127.0.0.1:8123
```

`mcp` and `tools call` print that same line. `config check` prints the sentence without the `load config:` prefix. `config check` rewrites several load errors. `mcp`, `tools`, and the wrapper do not.

The wrapper tarball contains `bin/index.js`, `package.json`, `README.md`, and `LICENSE`. The binary is in a platform package. npm installs the package that matches your OS and CPU. `0.2.1` publishes these packages:

| Package | OS | CPU |
| --- | --- | --- |
| `@futuretea/clickhouse-mcp-server-darwin-amd64` | darwin | x64 |
| `@futuretea/clickhouse-mcp-server-darwin-arm64` | darwin | arm64 |
| `@futuretea/clickhouse-mcp-server-linux-amd64` | linux | x64 |
| `@futuretea/clickhouse-mcp-server-linux-arm64` | linux | arm64 |
| `@futuretea/clickhouse-mcp-server-windows-amd64` | win32 | x64 |

`0.0.1` and `0.2.0` publish the same five packages. There is no npm `0.1.0`.

On another OS or CPU, the wrapper fails with `Could not resolve binary path for platform/architecture:` followed by the Node platform and architecture. The Windows binary file is `bin/clickhouse-mcp-server.exe`. The other platform packages use `bin/clickhouse-mcp-server`.

After `npm install @futuretea/clickhouse-mcp-server@0.2.1` on linux/amd64, this command prints the published build:

```bash
./node_modules/@futuretea/clickhouse-mcp-server-linux-amd64/bin/clickhouse-mcp-server version
```

The published linux/amd64 binary prints `version=0.2.1`, `commit=7a8469fee33eadf632ac3e4d3a964f7ea542d050`, and `date=2026-11-15T01:55:35-08:00`. The release workflow stamps that version and commit into each platform binary. A binary you build with `make build` on the `v0.2.1` tag prints `version=v0.2.1` because `git describe` keeps the `v`. The release workflow strips that prefix before it publishes npm.

### Docker image

`ghcr.io/futuretea/clickhouse-mcp-server:latest` is the same image index as `v0.2.1`. Runnable images are:

| Tag | Architectures |
| --- | --- |
| `v0.2.1`, `latest` | `linux/amd64`, `linux/arm64` |
| `v0.0.1`, `v0.1.0`, `v0.2.0` | `linux/amd64` |

The index also contains attestation manifests. Those entries are not images you can run.

The image entrypoint is `/usr/local/bin/clickhouse-mcp-server mcp`. The process user is `mcp`. The image includes CA certificates and does not include a config file. Arguments after the image name are flags for `mcp`.

To print the version, override the entrypoint:

```bash
docker run --rm --entrypoint /usr/local/bin/clickhouse-mcp-server \
  ghcr.io/futuretea/clickhouse-mcp-server:v0.2.1 version
```

`make docker` builds a local image for the Docker daemon's architecture. It tags `clickhouse-mcp-server:dev` and `clickhouse-mcp-server:<git describe>`. It does not push to GHCR and it does not build the published multi-architecture index.

The entrypoint and the process user come from the Dockerfile. The architecture table comes from the published image index.

### GitHub releases

[v0.2.1](https://github.com/futuretea/clickhouse-mcp-server/releases/tag/v0.2.1), [v0.2.0](https://github.com/futuretea/clickhouse-mcp-server/releases/tag/v0.2.0), and [v0.0.1](https://github.com/futuretea/clickhouse-mcp-server/releases/tag/v0.0.1) have release notes and no uploaded files. Do not look for an executable, a checksum, or a binary archive on those pages.

[v0.1.0](https://github.com/futuretea/clickhouse-mcp-server/tree/v0.1.0) is a Git tag and a `linux/amd64` GHCR image. It has no GitHub Release and no npm package.

To build this release from source, check out the tag. GitHub's automatic tag archive is source, not a built binary.

```bash
git clone https://github.com/futuretea/clickhouse-mcp-server.git
cd clickhouse-mcp-server
git checkout v0.2.1
```

### Build from source

The module requires Go 1.25.10.

```bash
make build
./bin/clickhouse-mcp-server version
```

On the `v0.2.1` commit, the version line contains `version=v0.2.1` and `commit=7a8469fee33eadf632ac3e4d3a964f7ea542d050`. The date is the time of that build. The source binary can run `version`, `config`, `tools`, `completion`, and `mcp`. The npm wrapper and the Docker entrypoint cannot. They always start `mcp`.

## Configure the connection

`clickhouse.address` is required. It is `host:port`, with no scheme, path, query, or userinfo. These values pass validation: `127.0.0.1:8123`, `clickhouse.local:8123`, and `[::1]:8123`. The port must be from 1 to 65535.

`config check` reports a missing address as:

```text
clickhouse.address is required; set a host:port such as 127.0.0.1:8123
```

`mcp` and `tools` report a missing address as:

```text
load config: clickhouse.address is required; set a host:port such as 127.0.0.1:8123
```

`config check` reports `http://127.0.0.1:8123` as `load config: clickhouse.address is invalid`. `mcp` and `tools` report that same address as `load config: clickhouse.address must be host:port without a URL scheme`. A path or userinfo is `load config: clickhouse.address must be a host:port` from `mcp` and `tools`, and `load config: clickhouse.address is invalid` from `config check`. A port outside 1 to 65535 is `load config: clickhouse.address must use a port from 1 to 65535` from `mcp` and `tools`, and the same invalid-address line from `config check`.

There is no `--address` flag and no connection URL field.

| YAML key | Environment variable | Default | Flag |
| --- | --- | --- | --- |
| `clickhouse.address` | `MCP_CLICKHOUSE_ADDRESS` | none | none |
| `clickhouse.database` | `MCP_CLICKHOUSE_DATABASE` | `default` | none |
| `clickhouse.secure` | `MCP_CLICKHOUSE_SECURE` | `false` | none |
| `clickhouse.username` | `MCP_CLICKHOUSE_USERNAME` | empty | none |
| `clickhouse.password` | `MCP_CLICKHOUSE_PASSWORD` | empty | none |
| `allow_unsafe_database_role` | `MCP_ALLOW_UNSAFE_DATABASE_ROLE` | `false` | none |

For a key that has a flag, an explicit flag overrides a non-empty environment variable. A flag you omit does not override anything. A non-empty environment variable overrides YAML. YAML overrides the default. An empty environment value does not count as set. `masking` does not follow this merge. See [Output masking](#output-masking).

`port`, `listen`, `sse_base_url`, and `log_level` have flags on the `mcp` command: `--port`, `--listen`, `--sse-base-url`, and `--log-level`. `config check` does not accept those flags. It still reads `MCP_PORT`, `MCP_LISTEN`, `MCP_SSE_BASE_URL`, and `MCP_LOG_LEVEL`.

An empty `clickhouse.database` becomes `default`, and `config check` reports `source=default`.

`secure: true` uses HTTPS to ClickHouse and requires TLS 1.2 or newer. It does not add TLS to the MCP HTTP port.

`clickhouse.secure` and `allow_unsafe_database_role` are booleans. The environment value is a string, and Viper decodes that string with `strconv.ParseBool`. The accepted true strings are `1`, `t`, `T`, `true`, `TRUE`, and `True`. The accepted false strings are `0`, `f`, `F`, `false`, `FALSE`, and `False`. `config check` accepts `1`, `t`, `T`, `true`, `TRUE`, `True`, `0`, `f`, `F`, `false`, `FALSE`, and `False` for `MCP_CLICKHOUSE_SECURE`. It accepts `1` for `MCP_ALLOW_UNSAFE_DATABASE_ROLE`. `yes`, `on`, and a value with surrounding spaces are rejected. `config check` exits 1 and prints `load config: configuration is invalid`. `mcp` and `tools` exit 1 and print `load config: unmarshal config: configuration is invalid`. An empty environment value is ignored. A YAML boolean `true` or `false` is accepted. A YAML value `yes` or `"yes"` is rejected. `config check` uses the generic configuration error. `mcp` uses the unmarshal error.

The `0.2.1` driver sends the ClickHouse user `default` when `clickhouse.username` is empty. A normal installation grants that user `ALL ON *.*`, and startup rejects it. Set `clickhouse.username` to a `SELECT`-only user. See [Service account](#service-account).

The server does not write the password to its logs. `config check` prints `clickhouse.password_status=set` or `clickhouse.password_status=unset`.

## Use stdio

stdio is the default. Leave `port` at `0`. The process disables logs so they do not mix with MCP messages. A startup warning from `allow_unsafe_database_role` is not printed in this mode.

Create the ClickHouse user in [Service account](#service-account) before you start the server. Then point a client that accepts an `mcpServers` map at the wrapper:

```json
{
  "mcpServers": {
    "clickhouse": {
      "command": "npx",
      "args": ["-y", "@futuretea/clickhouse-mcp-server@0.2.1"],
      "env": {
        "MCP_CLICKHOUSE_ADDRESS": "127.0.0.1:8123",
        "MCP_CLICKHOUSE_USERNAME": "mcp_reader"
      }
    }
  }
}
```

A client that starts a local binary passes `mcp` itself:

```json
{
  "mcpServers": {
    "clickhouse": {
      "command": "./bin/clickhouse-mcp-server",
      "args": ["mcp", "--config", "config.yaml"]
    }
  }
}
```

`config.example.yaml` sets `port: 0`. Copy it and set the address and username:

```bash
cp config.example.yaml config.yaml
```

The running process speaks MCP on stdin and stdout. `tools list` can show the catalog without a live server. `mcp` cannot. It opens ClickHouse before it reads stdin.

For a container, do not allocate a TTY. `127.0.0.1` inside the container is the container, not the host. Use an address the container can open.

```bash
docker run --rm -i \
  -e MCP_CLICKHOUSE_ADDRESS=clickhouse.internal:8123 \
  -e MCP_CLICKHOUSE_USERNAME=mcp_reader \
  ghcr.io/futuretea/clickhouse-mcp-server:v0.2.1
```

## Use HTTP

Set a non-zero port when the client needs a URL. The routes do not authenticate the caller.

1. Create the user in [Service account](#service-account).
2. Start the server on loopback.

   ```bash
   MCP_CLICKHOUSE_ADDRESS=127.0.0.1:8123 \
   MCP_CLICKHOUSE_USERNAME=mcp_reader \
   npx -y @futuretea/clickhouse-mcp-server@0.2.1 --port 8080
   ```

   A source build uses the same flags after `mcp`:

   ```bash
   ./bin/clickhouse-mcp-server mcp --config config.yaml --port 8080
   ```

3. Check the process.

   ```bash
   curl -sS http://127.0.0.1:8080/healthz
   ```

   `GET /healthz` returns status 200 and the body `healthy` after tools are registered. `HEAD /healthz` returns status 200 and an empty body. Its `Content-Length` is 7, the length of `healthy`. Any other method returns 405 and writes no body. `/healthz` does not ping ClickHouse. `config check --connect` does.

4. Point the client at Streamable HTTP.

   ```json
   {
     "mcpServers": {
       "clickhouse": {
         "url": "http://127.0.0.1:8080/mcp"
       }
     }
   }
   ```

The server does not read an `Authorization` header. Streamable HTTP is stateless. The HTTP integration test sends `initialize` and `tools/list` as separate requests.

| Path | Use |
| --- | --- |
| `/mcp` | Streamable HTTP |
| `/healthz` | Process check, `GET` and `HEAD` |
| `/sse` | Legacy SSE |
| `/message` | SSE message endpoint |

`sse_base_url` is the public base URL for the SSE message endpoint. `config check` removes the query and fragment and redacts URL userinfo before it prints that value. An unparseable value is printed as `configured`.

HTTP logs are JSON on stderr. The default level is `info`. Accepted levels are `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic`, and `disabled`. The server stops the HTTP listener on `SIGINT`, `SIGHUP`, or `SIGTERM`.

The read-header timeout is 5 seconds. The read timeout is 30 seconds. The write timeout is disabled so an open MCP stream is not cut off. The idle timeout is 120 seconds.

A published container port reaches the container interface, not `127.0.0.1` inside the container. Pass `--listen 0.0.0.0` when you publish the port. Bind the host side to loopback if the host should not expose the port. The MCP port still has no authentication.

```bash
docker run --rm -p 127.0.0.1:8080:8080 \
  -e MCP_CLICKHOUSE_ADDRESS=clickhouse.internal:8123 \
  -e MCP_CLICKHOUSE_USERNAME=mcp_reader \
  ghcr.io/futuretea/clickhouse-mcp-server:v0.2.1 \
  --port 8080 --listen 0.0.0.0
```

If you mount a file instead, the `mcp` user in the image must be able to read it. A mode `600` file owned by another user is not readable. Environment variables avoid that mount.

## Tools

Every tool belongs to the domain `clickhouse`. Each tool sets `readOnlyHint` true, `destructiveHint` false, and `idempotentHint` true. Those hints do not replace `readonly=1` or the startup privilege check.

A successful call returns one MCP text block. The text is JSON.

| Tool | Input | JSON result |
| --- | --- | --- |
| `clickhouse_list_databases` | none | `{"databases":["..."]}` |
| `clickhouse_list_tables` | optional `database` | `database`, and `tables` entries with `name`, `engine`, and `comment` |
| `clickhouse_describe_table` | required `table`, optional `database` | `database`, `table`, and `columns` entries with `name`, `type`, `default_kind`, `default_expression`, and `comment` |
| `clickhouse_query` | required `query`, optional `max_rows` | `columns`, `rows`, `row_count`, and `truncated` |

Omitted `database` uses `clickhouse.database`. `max_rows` defaults to 1000, and the schema minimum is 1. A value that is not a positive integer returns `max_rows must be a positive integer`. The query tool reads one extra row to set `truncated`. That extra row is not returned. `truncated` is present when it is false. List and describe do not apply `max_rows`.

`clickhouse_query` rows keep the driver display values. While masking is off, a non-null query cell is a string and a database NULL is JSON `null`. `clickhouse_list_databases`, `clickhouse_list_tables`, and `clickhouse_describe_table` return strings. A NULL name, engine, comment, type, default kind, or default expression becomes `""`. While masking is on, an unmatched query cell keeps that display text. A matched nested map or slice in a query row becomes JSON. A name rule replaces a matched string with the placeholder. In a query row, a matched non-string becomes JSON `null`. List and describe convert that null back to `""`.

`tools list` prints the four tools and does not connect. This command exits 0 even when the address refuses connections:

```bash
MCP_CLICKHOUSE_ADDRESS=127.0.0.1:8123 \
  ./bin/clickhouse-mcp-server tools list --json
```

`tools describe` and `tools describe <name> --json` also use the catalog only. `tools call` connects and runs the startup check.

`tools describe` and `tools call` reject a name that is missing or removed by the filters. The error includes that name. For the name `missing`, it is `unknown tool "missing"; run 'clickhouse-mcp-server tools list' to see available tools`. An MCP `tools/call` for a missing or filtered name is answered by the MCP library used by this server. That message is `tool 'missing' not found: tool not found`. It also includes the requested name. The CLI text and the MCP text are different.

An empty `enabled_tools` list means all tools. A non-empty list is an allowlist. `disabled_tools` then removes names. Domain filters use the same order for the domain `clickhouse`. If the filters remove every tool, `mcp` exits with:

```text
no tools registered; check enabled_tools / disabled_tools / domain filters
```

### Query text

`clickhouse_query` accepts one statement. A trailing semicolon is allowed. A semicolon inside quotes or comments belongs to that statement. A second statement returns `query must contain exactly one SQL statement`. An empty statement returns `query is required`. An open quote returns `query has an unterminated quoted value`. An open block comment returns `query has an unterminated block comment`. The server rejects these before it sends the text to ClickHouse.

Line comments start with `--` or `#`. Block comments use `/*` and `*/`.

Every tool query is sent with ClickHouse `readonly=1`. Startup probes, including `SHOW GRANTS FINAL`, are not sent with that setting. `readonly=1` is how the server asks ClickHouse to reject writes, DDL, `SYSTEM` statements, user administration, and source table functions such as `file` and `url`. It does not hide `system.processes`, and it does not reject an AI function by itself.

The integration test expects `SELECT 1 AS value` to return one string cell `"1"`, with `row_count` 1. `CREATE TEMPORARY TABLE` for the `SELECT` and `SHOW` account returns ClickHouse `ACCESS_DENIED` code 497. That privilege is global-only, and startup rejects a global grant, so the statement does not reach the `readonly=1` check.

## Configuration reference

`config.example.yaml` is the stdio shape. This HTTP shape leaves masking off:

```yaml
port: 8080
listen: 127.0.0.1
log_level: info
clickhouse:
  address: 127.0.0.1:8123
  database: default
  secure: false
  username: mcp_reader
  password: ""
allow_unsafe_database_role: false
```

| YAML key | Environment variable | Default |
| --- | --- | --- |
| `port` | `MCP_PORT` | `0`, which is stdio |
| `listen` | `MCP_LISTEN` | `127.0.0.1` |
| `sse_base_url` | `MCP_SSE_BASE_URL` | empty |
| `log_level` | `MCP_LOG_LEVEL` | `info` |
| `enabled_tools` | `MCP_ENABLED_TOOLS` | empty |
| `disabled_tools` | `MCP_DISABLED_TOOLS` | empty |
| `enabled_domains` | `MCP_ENABLED_DOMAINS` | empty |
| `disabled_domains` | `MCP_DISABLED_DOMAINS` | empty |
| `allow_unsafe_database_role` | `MCP_ALLOW_UNSAFE_DATABASE_ROLE` | `false` |
| `masking` | `MCP_MASKING_JSON` | disabled |

The root flags for tool filters are `--enabled-tools`, `--disabled-tools`, `--enable-domains`, and `--disable-domains`.

`config check` prints 15 fields in a fixed order, then the config path. Passwords appear only as `set` or `unset`. With only the address set, the output is:

```text
clickhouse.address=127.0.0.1:8123 source=environment
clickhouse.database=default source=default
clickhouse.secure=false source=default
clickhouse.username= source=default
clickhouse.password_status=unset source=default
port=0 source=default
listen=127.0.0.1 source=default
sse_base_url= source=default
log_level=info source=default
enabled_tools= source=default
disabled_tools= source=default
enabled_domains= source=default
disabled_domains= source=default
allow_unsafe_database_role=false source=default
masking=disabled source=default
config=(not set) source=default
```

Source labels are `flag`, `environment`, `YAML`, and `default`. The command does not open the network unless you pass `--connect`. `--connect` opens one HTTP or HTTPS connection, pings, and closes it. It does not run the privilege check. On success it prints `ClickHouse connection verified.` On failure it prints `ClickHouse ping failed:` and the driver text. A refused connection with a password shows the password as `***` in the URL, such as `http://default:***@127.0.0.1:1`. The server also replaces any remaining copy of the raw password, its query-escaped form, its path-escaped form, and the userinfo form with `[redacted]`. In that refused-connection text the password is already `***`, so `[redacted]` is not in the line.

```bash
./bin/clickhouse-mcp-server config check --connect
```

`mcp` and `tools call` do run the privilege check. `tools list` and `tools describe` do not.

## Output masking

Masking stays off when the `masking` key is absent, when `enabled` is false or omitted, and when the policy is the zero value. Tool results, logs, and errors then keep their normal text.

Set `enabled` to true and keep at least one rule. This object turns masking on:

```yaml
masking:
  enabled: true
  placeholder: "[masked]"
  names: ["synthetic_field"]
  name_patterns: ["token_.+"]
  values: ["synthetic-token-value"]
  value_patterns: ["sk_live_[0-9]+"]
```

`masking.ParseJSON` only parses one JSON document. It does not read the environment. `config.LoadConfig` does. When `MCP_MASKING_JSON` is present, `LoadConfig` passes that string to `ParseJSON`, and the parsed document replaces the whole YAML object. The value is one JSON object, not YAML. Other `MCP_*` variables do not merge into it. `MCP_MASKING_ENABLED` is ignored.

```bash
export MCP_MASKING_JSON='{"enabled":true,"values":["synthetic-token-value"]}'
```

An empty value, a second JSON value, an unknown field, or `enabled: true` with no rules is rejected. The error text is `invalid masking config`. The message does not include the document. An unset variable leaves the YAML object in place.

`config check` prints `masking=enabled` or `masking=disabled` and does not print the rules. The line is present while masking is off. When masking is on, displayed configuration values are masked too. The password status and the masking status itself are not masked as field values.

| Limit | Value |
| --- | --- |
| Placeholder | default `[masked]`, from 1 to 64 bytes |
| Entries in each of `names`, `name_patterns`, `values`, `value_patterns` | at most 32 |
| One name, one value literal, one pattern | at most 128, 256, and 128 bytes |
| Empty name, value, or pattern | rejected |
| Bytes counted for one masked result | 1,048,576 |
| Masking nodes, and nodes in the ClickHouse value walk | 10,000 each |
| JSON depth, and depth of the ClickHouse value walk | 32 each |
| Spans in one value | 4,096 |
| Times a JSON string is parsed again | 2 |

Name patterns are parsed on their own and then anchored to the whole field name. `a)|(b` is rejected. `(?:a)|(b)` matches only the whole names `a` and `b`. Value literals and value patterns are substring matches, including a one-character literal. RE2 returns the leftmost-first match and does not look for a longer alternative. The placeholder is not searched again.

A matched string becomes the placeholder. A matched number, boolean, or other non-string becomes null in the walk. An original null stays null. In a `clickhouse_query` row, that null stays JSON `null`, so it can be a database NULL or a redacted value, and the JSON type can change. List and describe store those fields as strings, so the null becomes `""`.

The server applies field rules to scanned driver values before `fmt.Sprint`. Unmatched cells keep the previous display text. ClickHouse does not decode a base64 string before masking it. A value rule that hits the original display text of a number, bool, time, duration, IP address, UUID, or geo value nulls the whole value. An unmatched value keeps that `fmt.Sprint` text. A JSON number keeps the digits from the document, including trailing zeros. A JSON value is counted again as its container, so it can exhaust the shared budget before a plain string of the same size. ClickHouse JSON and Variant values are walked. Two column names that mask to the same string fail the result.

While masking is off, this server does not apply that byte cap before it encodes the tool result. `max_rows` limits query rows, not bytes.

While masking is on, the cap applies to the masking walk. One input string longer than 1,048,576 bytes fails. The replacement calculated for one value cannot be longer than that. A rewritten JSON value fails when the rewritten JSON is longer than that. The bytes counted for the whole result cannot pass that total. A JSON value is counted again as its container. The final `json.Marshal` of the tool result is not measured a second time. An unknown value, a walk past a depth or node bound, or one of these byte checks fails the result with `[clickhouse_masking_failed] result was not returned`. No partial result is returned.

Masking does not find every private value. Compressed payloads and encodings the server does not recognize are not rewritten in place. Query text sent to ClickHouse is not modified.

While masking is on, query errors are `[clickhouse_query_failed] result was not returned`. Startup errors are `[clickhouse_startup_failed] ClickHouse startup check failed`. Connection errors from `config check --connect` are `[clickhouse_connect_failed] result was not returned`. An unrecognized AI setting is folded into the startup error. While masking is on, the process prints `[clickhouse_startup_failed] ClickHouse startup check failed` and omits the setting text. While masking is off, it prints `[clickhouse_startup_failed] ClickHouse startup check failed: unrecognized AI function setting "<text>"`, with the setting text in quotes. The query, startup, and `config check --connect` messages above omit driver and setting text while masking is on. While masking is on, a debug log for any request other than `/healthz` records `route` instead of `path`. The recorded class is `mcp`, `sse`, `message`, or `other`. `/healthz` is not logged. The placeholder and those fixed errors are not treated as stored data. A short literal that also occurs inside them can remain there.

## Service account

After the process connects, and before a tool runs a query, startup reads `SHOW GRANTS FINAL` for the current user. That statement includes privileges inherited through roles. The server expands roles for at most 8 rounds. The next round fails with `role grant expansion exceeded the startup limit`.

Startup exits with `[clickhouse_unsafe_service_role] database service role is not permitted` when the account matches any rule below. The full administrative and DDL strings are `alwaysUnsafePrivilegeNames` and `globalDDLPrivilegeNames` in [`pkg/toolset/clickhouse/startup.go`](pkg/toolset/clickhouse/startup.go).

Rejected at any scope:

- `ACCESS MANAGEMENT` and the user, role, policy, quota, profile, and token privileges in `alwaysUnsafePrivilegeNames`, including `CREATE TOKEN`, `IMPERSONATE`, and the `SQL SECURITY NONE` forms.
- `SYSTEM`, and any privilege whose name starts with `SYSTEM `.
- Source privileges: `SOURCES`, `READ`, `WRITE`, `AZURE`, `BIGQUERY`, `DISK`, `ELASTICSEARCH`, `FILE`, `HDFS`, `HIVE`, `JDBC`, `KAFKA`, `MONGO`, `MYSQL`, `NATS`, `ODBC`, `POSTGRES`, `RABBITMQ`, `REDIS`, `REMOTE`, `S3`, `SQLITE`, `URL`, `YTSAURUS`, `ARROW FLIGHT`, `ARROW_FLIGHT`, and `ARROWFLIGHT`.
- `SHOW CONNECTIONS`, `SHOW CONNECTION`, `SHOW HANDLERS`, `SHOW HANDLER`, `SHUTDOWN`, `BACKUP`, `KILL QUERY`, `KILL TRANSACTION`, `MOVE PARTITION BETWEEN SHARDS`, the named-collection privileges in that slice, `SET DEFINER`, `DISPLAYSECRETSINSHOWANDSELECT`, `TABLE ENGINE`, and `CLUSTER`.
- `AI FUNCTIONS`, and a single-token privilege whose normalized name has at least 3 characters and starts with `AI`, such as `aiGenerate`.

Rejected only on global `*.*`:

- `ALL`, `ALL PRIVILEGES`, `CREATE`, `ALTER`, `DROP`, `TRUNCATE`, `OPTIMIZE`, `UNDROP TABLE`, and each child in `globalDDLPrivilegeNames`, including `CREATE TEMPORARY TABLE` and `ALTER HANDLER`.

The same DDL privilege on one database does not fail this check. `INSERT` on one database does not fail it either. Tool queries still set `readonly=1`.

Rejected when `SELECT`, `ALL`, or `ALL PRIVILEGES` can read query text or account rows:

- `system.processes`
- `system.query_log`
- `system.query_thread_log`
- `system.query_views_log`
- `system.user_query_log`
- `system.query_cache`
- `system.asynchronous_inserts`
- `system.asynchronous_query_log`
- `system.users`

`SELECT` on `*.*` matches every one of those tables. An unquoted trailing `*` is a prefix, so `system.proc*`, `system.user*`, `system.a*`, `sys*.*`, and `s*.*` match. A backtick-quoted `*` is a literal name. The startup test allows `` GRANT SELECT ON system.`proc*` ``. Comparison is case sensitive, so `system.Processes` and `SYSTEM.processes` do not match `system.processes` in this check. `system.data*` matches `system.databases` and does not match `system.processes`, so that grant can start. `REVOKE SELECT ON system.processes` removes that exact table and leaves `GRANT SELECT ON *.*` in effect.

`SHOW` by itself does not match this query-text rule. `GRANT SHOW DATABASES`, `SHOW TABLES`, and `SHOW COLUMNS` on `*.*` can start. Column-level `SELECT(query, user) ON system.processes` is rejected.

Startup also reads `allow_experimental_ai_functions` from `system.settings`. A value of true rejects the account when `is_obsolete` is false. An obsolete true value does not reject it. An unknown setting does not reject it. A failed setting probe stops startup even when `allow_unsafe_database_role` is true. The startup tests record that ClickHouse 26.5.6.64, 26.5.7.64, and 26.10.2.103 reject `GRANT AI FUNCTIONS` as a syntax error. The unit tests still reject that grant text when a server returns it. The integration image is `clickhouse/clickhouse-server:26.5.6.64`. That script adds a live `AI FUNCTIONS` grant only when `system.privileges` contains the row.

`allow_unsafe_database_role` defaults to false. Set it to true in YAML, or export `MCP_ALLOW_UNSAFE_DATABASE_ROLE=true`, only when you cannot create a narrower account and you control the ClickHouse instance. There is no flag for this key. The server logs this warning and skips only the privilege rejection:

```text
database service role is not permitted; allow_unsafe_database_role is set, so the startup role check is skipped
```

The log level is `warn`. HTTP mode shows it at the default `info` level. A refused connection, a failed grant probe, a failed AI setting probe, and the role-expansion limit still stop startup. The flag does not remove `readonly=1`.

`config check --connect` does not run this check. `mcp` and `tools call` do.

Run this on ClickHouse as an administrator. It is not a statement for `clickhouse_query`.

```sql
CREATE USER mcp_reader IDENTIFIED WITH no_password;
GRANT SELECT ON analytics.* TO mcp_reader;
GRANT SELECT ON system.databases TO mcp_reader;
GRANT SELECT ON system.tables TO mcp_reader;
GRANT SELECT ON system.columns TO mcp_reader;
```

The discovery tools read those three system tables. Grant `SELECT` on each database the query tool should read. Replace `analytics` with that database.

## Command line

`clickhouse-mcp-server` with no arguments prints help and does not start the server. The subcommands are `mcp`, `config`, `tools` (alias `tool`), `version`, and `completion`.

```bash
./bin/clickhouse-mcp-server tools list --config config.yaml
./bin/clickhouse-mcp-server tools describe clickhouse_query --config config.yaml --json
./bin/clickhouse-mcp-server tools call clickhouse_list_databases --config config.yaml --params '{}'
./bin/clickhouse-mcp-server tools call clickhouse_query --config config.yaml \
  --params '{"query":"SELECT 1 AS value"}'
```

`tools describe` accepts the alias `schema`. `tools call` reads `--params` as a JSON object. The default is `{}`. `--params-file -` reads the object from stdin. `null` is rejected with `parse tool params JSON: expected an object`.

`completion` accepts `bash`, `zsh`, `fish`, `powershell`, and `install`.

## Develop and test

```bash
make format
make lint
make test
make build
make ci
```

`make ci` runs `lint`, `test`, and `build`. `lint` runs `go vet` and checks `gofmt` on `cmd`, `internal`, and `pkg`. It runs `golangci-lint` when that binary is installed. The GitHub Actions build workflow uses golangci-lint v2.12.2, runs `go test -race ./...`, and runs `make test-template-init`. `make ci` does not run the race detector or the integration script.

`make test` has no external service. `//go:build integration` keeps the ClickHouse tests out of that command. `make test-integration` needs Docker and `curl`. It starts `clickhouse/clickhouse-server:26.5.6.64` unless you set `CLICKHOUSE_INTEGRATION_IMAGE`, waits up to 45 seconds, and removes the container on exit. It creates `mcp_reader` and `wild_reader`, expects startup to reject `wild_reader`, and runs the HTTP end-to-end test, the toolset integration test, and `config check --connect`. The build workflow does not run it.

`scripts/` and `templates/` are retained template files. They are not the ClickHouse server runtime. `./scripts/init-template.sh` initializes a new repository created from the template. This repository is already initialized. Do not run that script as a build step. `make test-template-init` checks the script in a temporary copy. The build workflow runs that check.

Issues for this repository are tracked at <https://github.com/futuretea/clickhouse-mcp-server/issues>. Code and comments stay in English. Keep this file and [README.zh.md](README.zh.md) aligned. Platform names for a release live in [`.github/ci/release-platforms.json`](.github/ci/release-platforms.json). A stable tag `vX.Y.Z` runs the npm and Docker release workflows. The GitHub Release those workflows create has notes and no uploaded binary. Publishing npm requires the `NPM_TOKEN` secret.

## Troubleshoot

- `clickhouse.address is required`: this is `config check` with no address. Set `host:port`, or set `MCP_CLICKHOUSE_ADDRESS`. `mcp` and `tools` add the prefix `load config:`.
- `load config: clickhouse.address is invalid`: this is `config check` for an address that is not `host:port`. `mcp` and `tools` keep the specific validator text, such as `load config: clickhouse.address must be host:port without a URL scheme`.
- `load config: configuration is invalid`: this is `config check` for a missing file, a bad boolean, or other invalid configuration. The message does not quote the file. `mcp` and `tools` say `load config: read config file: configuration is invalid` for a missing file, and `load config: unmarshal config: configuration is invalid` for a bad boolean.
- `invalid masking config`: the masking document is empty, unknown, or enabled with no rules.
- `[clickhouse_unsafe_service_role]`: the account matches [Service account](#service-account). Use a `SELECT`-only user, or set `allow_unsafe_database_role` only on an instance you control.
- `[clickhouse_startup_failed] ClickHouse startup check failed:`: masking is off, and the driver text follows the colon. A refused connection includes `connection refused`. An empty username appears as the user `default`. A password appears as `***` in the URL, as in `http://default:***@127.0.0.1:1` from the 0.2.1 linux/amd64 binary. A raw password that is still present is replaced with `[redacted]`.
- `[clickhouse_startup_failed] ClickHouse startup check failed`: masking is on. There is no driver text.
- `[clickhouse_query_failed]`, `[clickhouse_connect_failed]`, and `[clickhouse_masking_failed]`: masking is on, or the result could not be masked. No partial tool result is returned. An AI setting failure is reported as `[clickhouse_startup_failed]`, not as a separate setting code.
- `ClickHouse ping failed:`: `config check --connect` could not ping. This check does not judge privileges.
- `truncated: true`: lower the result in SQL, or raise `max_rows` on purpose.
- `query must contain exactly one SQL statement`: send one statement.
- `no tools registered`: the tool or domain filters removed every tool.
- `Could not resolve binary path`: the OS and CPU are not one of the five published platform packages.

## License

[MIT](LICENSE)
