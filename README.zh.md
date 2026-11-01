# ClickHouse MCP Server

[English](README.md)

这是一个小型、默认只读的 ClickHouse MCP server。它通过 [`clickhouse-go/v2`](https://github.com/ClickHouse/clickhouse-go) 仅使用 HTTP(S) 连接 ClickHouse，并在 `/mcp` 提供 Streamable HTTP。

MCP 服务本身不做认证，默认只监听 `127.0.0.1`。除非另行设计认证和网络边界，否则只应在可信的本地网络中使用。

## 快速开始

1. 复制示例配置并设置 ClickHouse HTTP 地址。请使用只读用户；内置 `default` 用户会在启动时被拒绝。参见[服务账号](#服务账号)。

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

查询工具只接受一条 SQL。字符串、quoted identifier 和 SQL 注释中的分号可以使用；第二条语句会在到达 ClickHouse 前被拒绝。所有查询均携带 ClickHouse 的 `readonly=1` 设置，因此遵守此设置的 ClickHouse 会拒绝写入和 DDL。启动时还会拒绝可以管理 ClickHouse、读取外部数据源或执行全局 DDL 的账号。参见[服务账号](#服务账号)。

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
allow_unsafe_database_role: false
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
| 服务运行 | `allow_unsafe_database_role` | `MCP_ALLOW_UNSAFE_DATABASE_ROLE` | `false` | 仅跳过启动时的危险权限拒绝 |

服务不会记录配置的密码。`secure: true` 只保护到 ClickHouse 的连接；它不会为 MCP HTTP 端点增加 TLS 或认证。

### 服务账号

进程连上 ClickHouse 之后、任何工具执行查询之前，启动流程会读取当前用户的 `SHOW GRANTS FINAL`。该语句包含经由角色继承的权限。出现下列任一权限时，启动以 `[clickhouse_unsafe_service_role] database service role is not permitted` 失败：

- `ACCESS MANAGEMENT`，或其组内权限，包括 `CREATE USER`、`ALTER USER`、`DROP USER`、`CREATE ROLE`、`DROP ROLE`、`ROLE ADMIN`、行策略、配额、settings profile 和 `SHOW ACCESS`
- `SYSTEM`，或任何 `SYSTEM …` 权限，例如 `SYSTEM SHUTDOWN`
- `SOURCES`、`READ`、`WRITE`，或 `FILE`、`URL`、`REMOTE`、`S3`、`YTSAURUS`、`ARROW FLIGHT` 等外部数据源权限
- 作用在 `*.*` 上的全局 DDL，或全局 `ALL` / `ALL PRIVILEGES`：`CREATE`、`ALTER`、`DROP`、`TRUNCATE`、`OPTIMIZE` 及其 DDL 子权限，包括 `ALTER HANDLER`
- 类似的管理权限：`BACKUP`、`KILL QUERY`、`KILL TRANSACTION`、`MOVE PARTITION BETWEEN SHARDS`、`TABLE ENGINE`、`CLUSTER`、`NAMED COLLECTION ADMIN`（含 named collection 密钥）、`SET DEFINER`、`ALLOW SQL SECURITY NONE`、`SHOW CONNECTIONS` 和 `SHOW HANDLERS`

只有 `SELECT` 和 `SHOW` 的账号用默认配置即可启动，不需要打开开关。授予在单个数据库上、而不是 `*.*` 上的 DDL，不会被当成危险服务角色。查询仍然带 `readonly=1`。

`allow_unsafe_database_role` 默认为 `false`。只有无法准备最小权限账号时，才在配置文件里设为 `true`，或导出 `MCP_ALLOW_UNSAFE_DATABASE_ROLE=true`。服务会打一条警告日志，并跳过这一次权限拒绝。连接被拒绝、权限探测失败，以及其他启动检查仍会中止启动。`readonly=1` 约束不了 `SYSTEM`、`ACCESS MANAGEMENT` 或 `SOURCES`：这类账号可以关闭服务、修改用户，或在 `SELECT` 里读取服务器文件和远程 URL。只在你可控的 ClickHouse 实例上启用该选项。

警告使用 warn 级别。HTTP 模式在默认 `info` 日志级别下可以看到它。stdio 模式会关闭日志，以免干扰 MCP 输出。

`config check --connect` 只对 ClickHouse 做 `Ping`，不执行这次权限检查。`mcp` 和 `tools call` 会执行。

不要使用 ClickHouse 的 `default` 用户。常规安装会授予该用户 `ALL ON *.*`。

```sql
CREATE USER mcp_reader IDENTIFIED WITH no_password;
GRANT SELECT ON analytics.* TO mcp_reader;
GRANT SELECT ON system.databases TO mcp_reader;
GRANT SELECT ON system.tables TO mcp_reader;
GRANT SELECT ON system.columns TO mcp_reader;
```

内置的发现工具会读取这三张 system 表。查询工具需要读哪些库，就对那些库授予 `SELECT`。

### 仅用环境变量检查配置

最小本地路径只需要设置 ClickHouse HTTP 地址，然后查看生效配置。该命令不会启动 MCP，也不会访问网络。

```bash
export MCP_CLICKHOUSE_ADDRESS=127.0.0.1:8123
./bin/clickhouse-mcp-server config check
```

命令按稳定顺序输出 14 个配置字段及其来源（`flag`、`environment`、`YAML` 或 `default`）。密码只显示 `set` 或 `unset`；`sse_base_url` 在显示前会删除 query 和 fragment，并脱敏 URL userinfo。

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

## 发布制品

只有稳定的 `vX.Y.Z` Git tag 会触发发布。完成验证的稳定版本会更新 npm、Docker 和 GitHub Release 的 `latest` 指针；需要固定版本时请使用带版本号的引用。

```bash
npx -y @futuretea/clickhouse-mcp-server
npx -y @futuretea/clickhouse-mcp-server@<version>
docker run --rm -i ghcr.io/futuretea/clickhouse-mcp-server:latest
docker run --rm -i ghcr.io/futuretea/clickhouse-mcp-server:v<version>
```

两种方式都会启动 `mcp` 命令，包名或镜像名之后的参数会传递给它。维护者需要配置拥有 `@futuretea` 发布权限的 GitHub Actions `NPM_TOKEN`；首次发布后，请将 GHCR package 可见性设为公开。

## 排查

- `[clickhouse_unsafe_service_role]`：配置的用户可以管理 ClickHouse、读取外部数据源，或执行全局 DDL。请改用只有 `SELECT` 的用户，或仅在你可控的实例上设置 `allow_unsafe_database_role: true`。
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

默认测试不依赖外部服务。这个 opt-in 集成测试会启动隔离的本地 ClickHouse Docker 容器，验证数据库发现、带参数的表检查、`SELECT`，以及 `readonly=1` 会拒绝临时 DDL。它还会构建公开的 server 二进制文件，以 HTTP 模式在 loopback 上启动，检查 `/healthz`，并经由 `/mcp` 调用 `tools/list` 和 `tools/call`。

```bash
make test-integration
```

该目标需要 Docker 和 `curl`，会使用随机 loopback 端口、等待 ClickHouse 健康检查，并在退出时删除容器。可设置 `CLICKHOUSE_INTEGRATION_IMAGE` 来测试另一个隔离镜像。脚本会创建拥有 `*.*` 上 `SELECT` 和 `SHOW` 的用户 `mcp_reader`，并以此用户运行测试，因此启动权限检查在不设置 `allow_unsafe_database_role` 时可以通过。该目标会运行完整 MCP HTTP E2E、只读工具集集成测试和 `config check --connect`。它只在本地运行：`make test` 和 GitHub Actions CI 都不会执行它。

仓库仍保留 `scripts/` 和 `templates/` 中的模板资产供本地参考；它们不属于 ClickHouse server 的运行时部分。

## License

[MIT](LICENSE)
