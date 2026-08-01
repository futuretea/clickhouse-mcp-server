# ClickHouse MCP Server

[English](README.md)

这是一个小型、默认只读的 ClickHouse MCP server。它通过 [`clickhouse-go/v2`](https://github.com/ClickHouse/clickhouse-go) 仅使用 HTTP(S) 连接 ClickHouse，并在 `/mcp` 提供 Streamable HTTP。

MCP 服务本身不做认证，默认只监听 `127.0.0.1`。除非另行设计认证和网络边界，否则只应在可信的本地网络中使用。

## 快速开始

1. 复制示例配置并设置 ClickHouse HTTP 地址。

   ```bash
   cp config.example.yaml config.yaml
   ```

2. 构建并启动 HTTP 服务。

   ```bash
   make build
   ./bin/clickhouse-mcp-server mcp --config config.yaml --port 8080
   ```

3. 检查服务健康状态。

   ```bash
   curl -s http://127.0.0.1:8080/healthz
   # healthy
   ```

4. 将 MCP 客户端配置到 Streamable HTTP 端点。

   ```json
   {
     "mcpServers": {
       "clickhouse": {
         "url": "http://127.0.0.1:8080/mcp"
       }
     }
   }
   ```

## 工具

所有成功调用都在 MCP text content 中返回 JSON。

| 工具 | 输入 | 结果 |
|------|------|------|
| `clickhouse_list_databases` | 无 | `{"databases":["default"]}` |
| `clickhouse_list_tables` | 可选 `database` | 数据库名、表名、引擎和注释 |
| `clickhouse_describe_table` | 必填 `table`，可选 `database` | 列名、类型、默认值和注释 |
| `clickhouse_query` | 必填 `query`，可选正整数 `max_rows` | 列、显示字符串行、实际返回的 `row_count` 和 `truncated` |

`clickhouse_query` 默认最多返回 1,000 行。服务会额外读取一行判断 `truncated: true`，但该额外行不会出现在结果中。每个非空单元格都会显示为字符串，使 JSON 结构稳定。

查询工具只接受一条 SQL。字符串、quoted identifier 和 SQL 注释中的分号可以使用；第二条语句会在到达 ClickHouse 前被拒绝。所有查询均携带 ClickHouse 的 `readonly=1` 设置，因此遵守此设置的 ClickHouse 会拒绝写入和 DDL。连接使用的 ClickHouse 用户仍应只授予读所需的权限。

## 配置

优先级：**flags > 环境变量 > YAML 配置文件 > 默认值**。`clickhouse.address` 必填，格式必须是没有 scheme 或 path 的 `host:port`。

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

| 分组 | YAML 键 | 环境变量 | 默认值 | 含义 |
|------|----------|----------|--------|------|
| ClickHouse 连接 | `clickhouse.address` | `MCP_CLICKHOUSE_ADDRESS` | 无 | 必填的 ClickHouse `host:port` 地址 |
| ClickHouse 连接 | `clickhouse.database` | `MCP_CLICKHOUSE_DATABASE` | `default` | 表工具使用的默认数据库 |
| ClickHouse 连接 | `clickhouse.secure` | `MCP_CLICKHOUSE_SECURE` | `false` | 对 ClickHouse 使用 HTTPS 而非 HTTP |
| ClickHouse 连接 | `clickhouse.username` | `MCP_CLICKHOUSE_USERNAME` | 空 | 可选 ClickHouse 用户名 |
| ClickHouse 连接 | `clickhouse.password` | `MCP_CLICKHOUSE_PASSWORD` | 空 | 可选 ClickHouse 密码 |
| 服务运行 | `port` | `MCP_PORT` | `0` | HTTP 端口；`0` 启动 stdio 模式 |
| 服务运行 | `listen` | `MCP_LISTEN` | `127.0.0.1` | HTTP 监听地址 |
| 服务运行 | `sse_base_url` | `MCP_SSE_BASE_URL` | 空 | 兼容 SSE message 端点的公开基础 URL |
| 服务运行 | `log_level` | `MCP_LOG_LEVEL` | `info` | 服务日志级别 |
| 工具筛选 | `enabled_tools` | `MCP_ENABLED_TOOLS` | 空 | 以逗号分隔的允许工具名 |
| 工具筛选 | `disabled_tools` | `MCP_DISABLED_TOOLS` | 空 | 以逗号分隔的禁止工具名 |
| 工具筛选 | `enabled_domains` | `MCP_ENABLED_DOMAINS` | 空 | 以逗号分隔的允许工具域 |
| 工具筛选 | `disabled_domains` | `MCP_DISABLED_DOMAINS` | 空 | 以逗号分隔的禁止工具域 |

服务不会记录配置的密码。`secure: true` 只保护到 ClickHouse 的连接；它不会为 MCP HTTP 端点增加 TLS 或认证。

### 仅用环境变量检查配置

最小本地路径只需要设置 ClickHouse HTTP 地址，然后查看生效配置。该命令不会启动 MCP，也不会访问网络。

```bash
export MCP_CLICKHOUSE_ADDRESS=127.0.0.1:8123
./bin/clickhouse-mcp-server config check
```

命令按稳定顺序输出 13 个配置字段及其来源（`flag`、`environment`、`YAML` 或 `default`）。密码只显示 `set` 或 `unset`；`sse_base_url` 在显示前会删除 query 和 fragment，并脱敏 URL userinfo。

只有明确需要一次 ClickHouse HTTP(S) `Ping` 时才追加 `--connect`。它会打开、Ping 并关闭一个短生命周期连接，不会启动 MCP 服务。

```bash
./bin/clickhouse-mcp-server config check --connect
```

## CLI

CLI 使用同一份配置，可在没有 MCP 客户端时检查连通性。

```bash
./bin/clickhouse-mcp-server tools list --config config.yaml
./bin/clickhouse-mcp-server tools describe clickhouse_query --config config.yaml --json
./bin/clickhouse-mcp-server tools call clickhouse_list_databases --config config.yaml
./bin/clickhouse-mcp-server tools call clickhouse_query --config config.yaml \
  --params '{"query":"SELECT version()"}'
```

## 传输方式

当 `port` 为 `0` 时，`mcp` 启动 stdio；非零时启动 HTTP 服务。HTTP 模式提供：

| 路径 | 用途 |
|------|------|
| `/mcp` | 主要的 Streamable HTTP MCP 端点 |
| `/healthz` | 健康检查（`GET` / `HEAD`） |
| `/sse` | 兼容的 SSE MCP 端点 |
| `/message` | SSE message 端点 |

## Docker

```bash
make docker
docker run --rm -p 8080:8080 \
  -v "$PWD/config.yaml:/config.yaml:ro" \
  clickhouse-mcp-server:dev --config /config.yaml --port 8080
```

不要把无认证的 HTTP 端口直接暴露到公网。

## 排查

- `clickhouse.address is required`：在配置中填写 `host:port`，例如 `127.0.0.1:8123`，或设置 `MCP_CLICKHOUSE_ADDRESS`。
- 连接失败：检查地址、`secure` 值、ClickHouse HTTP 端口和网络可达性。
- 查询被拒绝：确认 SQL 只有一条只读语句，且 ClickHouse 用户有目标数据库的访问权限。
- `truncated: true`：在 SQL 中减少结果集，或有意识地提高 `max_rows`。

## 开发

```bash
make format
make lint
make test
make ci
```

### 可选 ClickHouse 集成测试

默认测试不依赖外部服务。这个 opt-in 集成测试会连接到配置的 ClickHouse HTTP 端点，验证数据库发现、`SELECT`，以及 `readonly=1` 会拒绝临时 DDL。请使用隔离实例：测试会有意尝试创建临时表，以证明只读设置确实生效。

```bash
docker run -d --rm --name clickhouse-mcp-integration \
  -e CLICKHOUSE_SKIP_USER_SETUP=1 \
  -p 127.0.0.1:8123:8123 \
  clickhouse/clickhouse-server:26.5.6.64

MCP_CLICKHOUSE_ADDRESS=127.0.0.1:8123 make test-integration
docker stop clickhouse-mcp-integration
```

若隔离 ClickHouse 使用其他配置，可设置 `MCP_CLICKHOUSE_DATABASE`、`MCP_CLICKHOUSE_SECURE`、`MCP_CLICKHOUSE_USERNAME` 和 `MCP_CLICKHOUSE_PASSWORD`。该目标会同时运行只读工具集集成测试和 `config check --connect`；缺少 `MCP_CLICKHOUSE_ADDRESS` 时会失败，不会静默跳过集成检查。

仓库仍保留 `scripts/` 和 `templates/` 中的模板资产供本地参考；它们不属于 ClickHouse server 的运行时部分。

## License

[MIT](LICENSE)
