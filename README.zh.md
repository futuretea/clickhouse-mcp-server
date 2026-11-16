# ClickHouse MCP Server

[English](README.md)

这个服务器让 MCP 客户端列出 ClickHouse 的数据库和表，并执行一条只读 SQL。它通过 [`clickhouse-go` v2](https://github.com/ClickHouse/clickhouse-go) 使用 HTTP 或 HTTPS 连接 ClickHouse。它不打开 ClickHouse 的原生 TCP 端口。

`port` 为 `0` 时，`mcp` 使用 stdio。这是默认值。非零端口启动 HTTP。HTTP 路由不检查调用方凭证。默认监听地址是 `127.0.0.1`。请把这个端口留在可信网络里，或在它前面加上你自己的认证和网络边界。

当前 npm 的 `latest` 和 GHCR 的 `latest` 都是 `0.2.1`。

## 安装当前版本

### npm

安装一个固定版本：

```bash
npx -y @futuretea/clickhouse-mcp-server@0.2.1 --help
```

进程打印 `mcp` 的帮助并退出，退出码为 0。[npm 上的包](https://www.npmjs.com/package/@futuretea/clickhouse-mcp-server)是包装器。它的 `bin` 入口会运行 `clickhouse-mcp-server mcp`，再接上你传入的参数。这些参数是 `mcp` 的标志，例如 `--port`。它们不是子命令。

```bash
npx -y @futuretea/clickhouse-mcp-server@0.2.1 version
```

这不会打印版本。包装器仍会启动 `mcp`。没有 ClickHouse 地址时，进程以退出码 1 结束，并打印：

```text
clickhouse.address is required; set a host:port such as 127.0.0.1:8123
```

包装器的 tarball 包含 `bin/index.js`、`package.json`、`README.md` 和 `LICENSE`。二进制文件在平台包里。npm 会安装与你的操作系统和 CPU 匹配的包。`0.2.1` 发布了这些包：

| 包 | 操作系统 | CPU |
| --- | --- | --- |
| `@futuretea/clickhouse-mcp-server-darwin-amd64` | darwin | x64 |
| `@futuretea/clickhouse-mcp-server-darwin-arm64` | darwin | arm64 |
| `@futuretea/clickhouse-mcp-server-linux-amd64` | linux | x64 |
| `@futuretea/clickhouse-mcp-server-linux-arm64` | linux | arm64 |
| `@futuretea/clickhouse-mcp-server-windows-amd64` | win32 | x64 |

`0.0.1` 和 `0.2.0` 发布同样的五个包。没有 npm `0.1.0`。

在其他操作系统或 CPU 上，包装器失败，错误以 `Could not resolve binary path for platform/architecture:` 开头，后面是 Node 的平台和架构。Windows 的二进制文件是 `bin/clickhouse-mcp-server.exe`。其他平台包使用 `bin/clickhouse-mcp-server`。

在 linux/amd64 上执行 `npm install @futuretea/clickhouse-mcp-server@0.2.1` 之后，这条命令打印已发布的构建：

```bash
./node_modules/@futuretea/clickhouse-mcp-server-linux-amd64/bin/clickhouse-mcp-server version
```

已发布的 linux/amd64 二进制文件打印 `version=0.2.1`、`commit=7a8469fee33eadf632ac3e4d3a964f7ea542d050` 和 `date=2026-11-15T01:55:35-08:00`。发布工作流会把这个版本和提交写入每个平台二进制文件。在 `v0.2.1` 标签上用 `make build` 构建的二进制文件会打印 `version=v0.2.1`，因为 `git describe` 保留了 `v`。发布工作流在发布 npm 之前会去掉这个前缀。

### Docker 镜像

`ghcr.io/futuretea/clickhouse-mcp-server:latest` 与 `v0.2.1` 是同一个镜像索引。可运行的镜像是：

| 标签 | 架构 |
| --- | --- |
| `v0.2.1`、`latest` | `linux/amd64`、`linux/arm64` |
| `v0.0.1`、`v0.1.0`、`v0.2.0` | `linux/amd64` |

索引里还有证明清单。那些条目不是可以运行的镜像。

镜像入口点是 `/usr/local/bin/clickhouse-mcp-server mcp`。进程用户是 `mcp`。镜像包含 CA 证书，不包含配置文件。镜像名后面的参数是 `mcp` 的标志。

要打印版本，请覆盖入口点：

```bash
docker run --rm --entrypoint /usr/local/bin/clickhouse-mcp-server \
  ghcr.io/futuretea/clickhouse-mcp-server:v0.2.1 version
```

`make docker` 按 Docker 守护进程的架构构建本地镜像。它打上 `clickhouse-mcp-server:dev` 和 `clickhouse-mcp-server:<git describe>` 两个标签。它不会推送到 GHCR，也不会构建已发布的多架构索引。

入口点和进程用户来自 Dockerfile。架构表来自已发布的镜像索引。

### GitHub Release

[v0.2.1](https://github.com/futuretea/clickhouse-mcp-server/releases/tag/v0.2.1)、[v0.2.0](https://github.com/futuretea/clickhouse-mcp-server/releases/tag/v0.2.0) 和 [v0.0.1](https://github.com/futuretea/clickhouse-mcp-server/releases/tag/v0.0.1) 有发布说明，没有上传的文件。不要在这些页面上寻找可执行文件、校验和或二进制归档。

[v0.1.0](https://github.com/futuretea/clickhouse-mcp-server/tree/v0.1.0) 是一个 Git 标签，也是一个 `linux/amd64` 的 GHCR 镜像。它没有 GitHub Release，也没有 npm 包。

要从源码构建这个版本，请检出该标签。GitHub 为标签自动生成的归档是源码，不是构建好的二进制文件。

```bash
git clone https://github.com/futuretea/clickhouse-mcp-server.git
cd clickhouse-mcp-server
git checkout v0.2.1
```

### 从源码构建

这个模块需要 Go 1.25.10。

```bash
make build
./bin/clickhouse-mcp-server version
```

在 `v0.2.1` 这个提交上，版本行包含 `version=v0.2.1` 和 `commit=7a8469fee33eadf632ac3e4d3a964f7ea542d050`。日期是那次构建的时间。源码二进制文件可以运行 `version`、`config`、`tools`、`completion` 和 `mcp`。npm 包装器和 Docker 入口点不能。它们总会启动 `mcp`。

## 配置连接

`clickhouse.address` 必填。它的格式是 `host:port`，不能带 scheme、path、query 或 userinfo。这些值可以通过校验：`127.0.0.1:8123`、`clickhouse.local:8123` 和 `[::1]:8123`。端口必须在 1 到 65535 之间。

缺少地址时，`config check` 报告：

```text
clickhouse.address is required; set a host:port such as 127.0.0.1:8123
```

不是 `host:port` 的地址，包括 `http://127.0.0.1:8123`，会被报告为：

```text
load config: clickhouse.address is invalid
```

没有 `--address` 标志，也没有连接 URL 字段。

| YAML 键 | 环境变量 | 默认值 | 标志 |
| --- | --- | --- | --- |
| `clickhouse.address` | `MCP_CLICKHOUSE_ADDRESS` | 无 | 无 |
| `clickhouse.database` | `MCP_CLICKHOUSE_DATABASE` | `default` | 无 |
| `clickhouse.secure` | `MCP_CLICKHOUSE_SECURE` | `false` | 无 |
| `clickhouse.username` | `MCP_CLICKHOUSE_USERNAME` | 空 | 无 |
| `clickhouse.password` | `MCP_CLICKHOUSE_PASSWORD` | 空 | 无 |
| `allow_unsafe_database_role` | `MCP_ALLOW_UNSAFE_DATABASE_ROLE` | `false` | 无 |

有标志的键，显式标志覆盖非空环境变量。你省略的标志不会覆盖任何值。非空环境变量覆盖 YAML。YAML 覆盖默认值。空的环境变量值不算已设置。`masking` 不按这个规则合并。参见[输出脱敏](#输出脱敏)。

`port`、`listen`、`sse_base_url` 和 `log_level` 在 `mcp` 命令上有标志：`--port`、`--listen`、`--sse-base-url` 和 `--log-level`。`config check` 不接受这些标志。它仍然读取 `MCP_PORT`、`MCP_LISTEN`、`MCP_SSE_BASE_URL` 和 `MCP_LOG_LEVEL`。

空的 `clickhouse.database` 会变成 `default`，`config check` 报告 `source=default`。

`secure: true` 对 ClickHouse 使用 HTTPS，并要求 TLS 1.2 或更新的版本。它不会给 MCP 的 HTTP 端口加 TLS。

`clickhouse.secure` 和 `allow_unsafe_database_role` 是布尔值。环境变量的值是字符串，Viper 用 `strconv.ParseBool` 解码这个字符串。接受的 true 字符串是 `1`、`t`、`T`、`true`、`TRUE` 和 `True`。接受的 false 字符串是 `0`、`f`、`F`、`false`、`FALSE` 和 `False`。`config check` 对 `MCP_CLICKHOUSE_SECURE` 接受 `1`、`t`、`T`、`true`、`TRUE`、`True`、`0`、`f`、`F`、`false`、`FALSE` 和 `False`。它对 `MCP_ALLOW_UNSAFE_DATABASE_ROLE` 接受 `1`。`yes`、`on` 和带前后空格的值会被拒绝。进程以退出码 1 结束，并打印 `load config: configuration is invalid`。空的环境变量值会被忽略。YAML 布尔值 `true` 或 `false` 可以接受。YAML 值 `yes` 或 `"yes"` 会以同一个错误被拒绝。

`clickhouse.username` 为空时，`0.2.1` 的驱动会发送 ClickHouse 用户 `default`。常规安装会授予该用户 `ALL ON *.*`，启动会拒绝它。请把 `clickhouse.username` 设成只有 `SELECT` 的用户。参见[服务账号](#服务账号)。

服务器不会把密码写入日志。`config check` 打印 `clickhouse.password_status=set` 或 `clickhouse.password_status=unset`。

## 使用 stdio

stdio 是默认方式。让 `port` 保持 `0`。进程会关闭日志，以免日志混进 MCP 消息。这个模式下不会打印 `allow_unsafe_database_role` 的启动警告。

先按[服务账号](#服务账号)创建 ClickHouse 用户，再启动服务器。然后把接受 `mcpServers` 映射的客户端指向包装器：

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

启动本地二进制文件的客户端需要自己传入 `mcp`：

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

`config.example.yaml` 把 `port` 设为 `0`。复制它，再设置地址和用户名：

```bash
cp config.example.yaml config.yaml
```

运行中的进程在 stdin 和 stdout 上使用 MCP。`tools list` 可以在没有活动服务器的情况下显示工具目录。`mcp` 不能。它会先打开 ClickHouse，然后才读取 stdin。

使用容器时，不要分配 TTY。容器里的 `127.0.0.1` 是容器自己，不是宿主机。请使用容器能够打开的地址。

```bash
docker run --rm -i \
  -e MCP_CLICKHOUSE_ADDRESS=clickhouse.internal:8123 \
  -e MCP_CLICKHOUSE_USERNAME=mcp_reader \
  ghcr.io/futuretea/clickhouse-mcp-server:v0.2.1
```

## 使用 HTTP

客户端需要 URL 时，设置一个非零端口。这些路由不认证调用方。

1. 按[服务账号](#服务账号)创建用户。
2. 在回环地址上启动服务器。

   ```bash
   MCP_CLICKHOUSE_ADDRESS=127.0.0.1:8123 \
   MCP_CLICKHOUSE_USERNAME=mcp_reader \
   npx -y @futuretea/clickhouse-mcp-server@0.2.1 --port 8080
   ```

   源码构建在 `mcp` 后面使用同样的标志：

   ```bash
   ./bin/clickhouse-mcp-server mcp --config config.yaml --port 8080
   ```

3. 检查进程。

   ```bash
   curl -sS http://127.0.0.1:8080/healthz
   ```

   工具注册之后，`GET /healthz` 返回状态 200，正文是 `healthy`。`HEAD /healthz` 返回状态 200，正文为空。它的 `Content-Length` 是 7，也就是 `healthy` 的长度。其他方法返回 405，并且不写正文。`/healthz` 不会 ping ClickHouse。`config check --connect` 会。

4. 把客户端指向 Streamable HTTP。

   ```json
   {
     "mcpServers": {
       "clickhouse": {
         "url": "http://127.0.0.1:8080/mcp"
       }
     }
   }
   ```

服务器不读取 `Authorization` 头。Streamable HTTP 是无状态的。HTTP 集成测试把 `initialize` 和 `tools/list` 作为分开的请求发送。

| 路径 | 用途 |
| --- | --- |
| `/mcp` | Streamable HTTP |
| `/healthz` | 进程检查，`GET` 和 `HEAD` |
| `/sse` | 旧的 SSE |
| `/message` | SSE 消息端点 |

`sse_base_url` 是 SSE 消息端点的公开基础 URL。`config check` 在打印前会去掉 query 和 fragment，并脱敏 URL userinfo。无法解析的值会打印成 `configured`。

HTTP 日志是写到 stderr 的 JSON。默认级别是 `info`。接受的级别是 `trace`、`debug`、`info`、`warn`、`error`、`fatal`、`panic` 和 `disabled`。服务器在收到 `SIGINT`、`SIGHUP` 或 `SIGTERM` 时停止 HTTP 监听。

读取请求头的超时是 5 秒。读取请求的超时是 30 秒。写超时被关掉，以免切断已经打开的 MCP 流。空闲超时是 120 秒。

发布出来的容器端口到达的是容器网卡，不是容器内部的 `127.0.0.1`。发布端口时请传入 `--listen 0.0.0.0`。如果宿主机不该暴露这个端口，就把宿主机一侧绑定到回环地址。MCP 端口仍然没有认证。

```bash
docker run --rm -p 127.0.0.1:8080:8080 \
  -e MCP_CLICKHOUSE_ADDRESS=clickhouse.internal:8123 \
  -e MCP_CLICKHOUSE_USERNAME=mcp_reader \
  ghcr.io/futuretea/clickhouse-mcp-server:v0.2.1 \
  --port 8080 --listen 0.0.0.0
```

如果改为挂载文件，镜像里的 `mcp` 用户必须能够读取它。其他用户拥有的 `600` 权限文件不可读。用环境变量就可以避免这个挂载。

## 工具

每个工具都属于域 `clickhouse`。每个工具都把 `readOnlyHint` 设为 true，把 `destructiveHint` 设为 false，把 `idempotentHint` 设为 true。这些提示不能代替 `readonly=1` 或启动时的权限检查。

成功的调用返回一个 MCP 文本块。文本是 JSON。

| 工具 | 输入 | JSON 结果 |
| --- | --- | --- |
| `clickhouse_list_databases` | 无 | `{"databases":["..."]}` |
| `clickhouse_list_tables` | 可选 `database` | `database`，以及带 `name`、`engine` 和 `comment` 的 `tables` 条目 |
| `clickhouse_describe_table` | 必填 `table`，可选 `database` | `database`、`table`，以及带 `name`、`type`、`default_kind`、`default_expression` 和 `comment` 的 `columns` 条目 |
| `clickhouse_query` | 必填 `query`，可选 `max_rows` | `columns`、`rows`、`row_count` 和 `truncated` |

省略 `database` 时使用 `clickhouse.database`。`max_rows` 默认为 1000，schema 的最小值是 1。不是正整数的值会返回 `max_rows must be a positive integer`。查询工具会多读一行来设置 `truncated`。这一行不会返回。`truncated` 为 false 时仍然出现。列出和描述工具不使用 `max_rows`。

脱敏关闭时，非空单元格是字符串。数据库 NULL 是 JSON `null`。脱敏打开时，未命中的单元格保持该显示文本。命中的嵌套 map 或 slice 变成 JSON。名字规则把命中的字符串换成占位符，并把其他命中的类型换成 JSON `null`。

`tools list` 打印这四个工具，并且不连接。地址拒绝连接时，这条命令仍然以退出码 0 结束：

```bash
MCP_CLICKHOUSE_ADDRESS=127.0.0.1:8123 \
  ./bin/clickhouse-mcp-server tools list --json
```

`tools describe` 和 `tools describe <name> --json` 也只使用工具目录。`tools call` 会连接并执行启动检查。

`tools describe` 和 `tools call` 会拒绝缺失的名字，以及被过滤去掉的名字。错误里包含这个名字。对于名字 `missing`，错误是 `unknown tool "missing"; run 'clickhouse-mcp-server tools list' to see available tools`。对缺失或被过滤名字的 MCP `tools/call` 由本服务器使用的 MCP 库回答。那条消息是 `tool 'missing' not found: tool not found`。它也包含请求的名字。CLI 文本和 MCP 文本不一样。

空的 `enabled_tools` 表示全部工具。非空列表是允许列表。`disabled_tools` 再按名字去掉工具。域过滤对域 `clickhouse` 使用同样的顺序。如果过滤后一个工具都不剩，`mcp` 退出并打印：

```text
no tools registered; check enabled_tools / disabled_tools / domain filters
```

### 查询文本

`clickhouse_query` 接受一条语句。末尾的分号可以使用。引号或注释里的分号属于这一条语句。第二条语句返回 `query must contain exactly one SQL statement`。空语句返回 `query is required`。未结束的引号返回 `query has an unterminated quoted value`。未结束的块注释返回 `query has an unterminated block comment`。服务器在把文本发给 ClickHouse 之前拒绝这些输入。

行注释以 `--` 或 `#` 开头。块注释使用 `/*` 和 `*/`。

每个工具查询都带 ClickHouse 的 `readonly=1`。启动探测不带这个设置，包括 `SHOW GRANTS FINAL`。`readonly=1` 是服务器要求 ClickHouse 拒绝写入、DDL、`SYSTEM` 语句、用户管理，以及 `file` 和 `url` 这类外部数据源表函数的方式。它藏不住 `system.processes`，也不会单独拒绝 AI 函数。

集成测试期望 `SELECT 1 AS value` 返回一个字符串单元格 `"1"`，并且 `row_count` 为 1。对只有 `SELECT` 和 `SHOW` 的账号，`CREATE TEMPORARY TABLE` 返回 ClickHouse 的 `ACCESS_DENIED`，代码是 497。该权限只能授在全局范围，启动会拒绝全局授权，因此这条语句到不了 `readonly=1` 检查。

## 配置参考

`config.example.yaml` 是 stdio 的形状。下面这个 HTTP 形状把脱敏留在关闭状态：

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

| YAML 键 | 环境变量 | 默认值 |
| --- | --- | --- |
| `port` | `MCP_PORT` | `0`，也就是 stdio |
| `listen` | `MCP_LISTEN` | `127.0.0.1` |
| `sse_base_url` | `MCP_SSE_BASE_URL` | 空 |
| `log_level` | `MCP_LOG_LEVEL` | `info` |
| `enabled_tools` | `MCP_ENABLED_TOOLS` | 空 |
| `disabled_tools` | `MCP_DISABLED_TOOLS` | 空 |
| `enabled_domains` | `MCP_ENABLED_DOMAINS` | 空 |
| `disabled_domains` | `MCP_DISABLED_DOMAINS` | 空 |
| `allow_unsafe_database_role` | `MCP_ALLOW_UNSAFE_DATABASE_ROLE` | `false` |
| `masking` | `MCP_MASKING_JSON` | 关闭 |

工具过滤的根标志是 `--enabled-tools`、`--disabled-tools`、`--enable-domains` 和 `--disable-domains`。

`config check` 按固定顺序打印 15 个字段，然后打印配置路径。密码只显示为 `set` 或 `unset`。只设置地址时，输出是：

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

来源标签是 `flag`、`environment`、`YAML` 和 `default`。除非你传入 `--connect`，否则命令不会打开网络。`--connect` 打开一次 HTTP 或 HTTPS 连接，执行 ping，然后关闭。它不运行权限检查。成功时打印 `ClickHouse connection verified.`。失败时打印 `ClickHouse ping failed:` 和驱动文本，已配置的密码会被换成 `[redacted]`。

```bash
./bin/clickhouse-mcp-server config check --connect
```

`mcp` 和 `tools call` 会运行权限检查。`tools list` 和 `tools describe` 不会。

## 输出脱敏

`masking` 键不存在、`enabled` 为 false 或被省略、或策略是零值时，脱敏保持关闭。工具结果、日志和错误这时保持原来的文本。

把 `enabled` 设为 true，并至少保留一条规则。这个对象会打开脱敏：

```yaml
masking:
  enabled: true
  placeholder: "[masked]"
  names: ["synthetic_field"]
  name_patterns: ["token_.+"]
  values: ["synthetic-token-value"]
  value_patterns: ["sk_live_[0-9]+"]
```

`masking.ParseJSON` 只解析一个 JSON 文档。它不读取环境变量。读取环境变量的是 `config.LoadConfig`。`MCP_MASKING_JSON` 存在时，`LoadConfig` 把这个字符串传给 `ParseJSON`，解析后的文档替换整个 YAML 对象。这个值是一个 JSON 对象，不是 YAML。其他 `MCP_*` 变量不会并入它。`MCP_MASKING_ENABLED` 会被忽略。

```bash
export MCP_MASKING_JSON='{"enabled":true,"values":["synthetic-token-value"]}'
```

空值、第二个 JSON 值、未知字段，或 `enabled: true` 但没有任何规则，都会被拒绝。错误文本是 `invalid masking config`。这条消息不包含配置文档。未设置该变量时，保留 YAML 对象。

`config check` 打印 `masking=enabled` 或 `masking=disabled`，不打印规则。脱敏关闭时这一行仍在。脱敏打开时，配置检查里显示的配置值也会被脱敏。密码状态和脱敏状态本身不会被当作字段值来脱敏。

| 限制 | 值 |
| --- | --- |
| 占位符 | 默认 `[masked]`，长度 1 到 64 字节 |
| `names`、`name_patterns`、`values`、`value_patterns` 各自的条目数 | 最多 32 |
| 一个名字、一个值字面量、一个模式 | 最多 128、256 和 128 字节 |
| 空的名字、值或模式 | 拒绝 |
| 一次脱敏结果计入的字节 | 1,048,576 |
| 脱敏节点数，以及 ClickHouse 值遍历的节点数 | 各 10,000 |
| JSON 深度，以及 ClickHouse 值遍历的深度 | 各 32 |
| 一个值里的 span 数 | 4,096 |
| JSON 字符串再次解析的次数 | 2 |

名字模式会先单独解析，再锚定到整个字段名。`a)|(b` 会被拒绝。`(?:a)|(b)` 只匹配完整名字 `a` 和 `b`。值字面量和值模式是子串匹配，包括单字符字面量。RE2 返回最左优先的匹配，不会再找更长的分支。占位符不会被再次搜索。

命中的字符串变成占位符。命中的数字、布尔值或其他非字符串变成 JSON `null`。原来的 null 仍是 null。结果里的 null 可能是数据库 NULL，也可能是被抹掉的值，而且 JSON 类型可能改变。

服务器在 `fmt.Sprint` 之前对驱动扫描值应用字段规则。未命中的单元格保持原来的显示文本。ClickHouse 不会在脱敏前解码 base64 字符串。值规则命中数字、布尔值、时间、时长、IP 地址、UUID 或地理值的原始显示文本时，整个值变成 null。未命中的值保持原来的 `fmt.Sprint` 文本。JSON 数字保留文档里的数字，包括末尾的 0。JSON 值会再按容器计一次，因此同样大小的 JSON 可能比普通字符串更早用完共享预算。ClickHouse 的 JSON 和 Variant 会被遍历。两个列名脱敏成同一个字符串时，结果失败。

脱敏关闭时，服务器在编码工具结果之前不使用这个字节上限。`max_rows` 限制的是查询行数，不是字节。

脱敏打开时，这个上限作用于脱敏遍历。超过 1,048,576 字节的单个输入字符串会失败。为一个值算出的替换结果不能比这更长。改写后的 JSON 值在改写结果更长时失败。整次结果计入的字节不能超过这个总数。JSON 值会再按容器计一次。工具结果最终的 `json.Marshal` 不会被再量一次。未知类型、超过深度或节点上限，或上述任一字节检查失败时，结果失败，错误为 `[clickhouse_masking_failed] result was not returned`。不会返回部分结果。

脱敏找不到所有私密值。压缩数据和服务器不认识的编码不会被就地改写。发给 ClickHouse 的查询文本不会被修改。

脱敏打开时，查询错误是 `[clickhouse_query_failed] result was not returned`。启动错误是 `[clickhouse_startup_failed] ClickHouse startup check failed`。`config check --connect` 的连接错误是 `[clickhouse_connect_failed] result was not returned`。无法识别的 AI 设置值是 `[clickhouse_setting_failed] result was not returned`。这些消息省略驱动和设置文本。脱敏关闭时，无法识别的 AI 设置错误会包含设置文本。脱敏打开时，除 `/healthz` 以外的请求，调试日志记录 `route` 而不是 `path`。记录的类别是 `mcp`、`sse`、`message` 或 `other`。`/healthz` 不会被记录。占位符和这些固定错误不当作存储的数据。占位符或固定错误里碰巧包含的短字面量仍可能留在那里。

## 服务账号

进程连上 ClickHouse 之后、任何工具执行查询之前，启动流程读取当前用户的 `SHOW GRANTS FINAL`。该语句包含经角色继承的权限。服务器最多展开 8 轮角色。下一轮失败，错误是 `role grant expansion exceeded the startup limit`。

账号命中下面任一规则时，启动以 `[clickhouse_unsafe_service_role] database service role is not permitted` 退出。完整的管理和 DDL 字符串是 [`pkg/toolset/clickhouse/startup.go`](pkg/toolset/clickhouse/startup.go) 里的 `alwaysUnsafePrivilegeNames` 和 `globalDDLPrivilegeNames`。

在任何范围都会拒绝：

- `ACCESS MANAGEMENT`，以及 `alwaysUnsafePrivilegeNames` 里的用户、角色、策略、配额、配置档案和令牌权限，包括 `CREATE TOKEN`、`IMPERSONATE` 和 `SQL SECURITY NONE` 的几种写法。
- `SYSTEM`，以及名字以 `SYSTEM ` 开头的任何权限。
- 数据源权限：`SOURCES`、`READ`、`WRITE`、`AZURE`、`BIGQUERY`、`DISK`、`ELASTICSEARCH`、`FILE`、`HDFS`、`HIVE`、`JDBC`、`KAFKA`、`MONGO`、`MYSQL`、`NATS`、`ODBC`、`POSTGRES`、`RABBITMQ`、`REDIS`、`REMOTE`、`S3`、`SQLITE`、`URL`、`YTSAURUS`、`ARROW FLIGHT`、`ARROW_FLIGHT` 和 `ARROWFLIGHT`。
- `SHOW CONNECTIONS`、`SHOW CONNECTION`、`SHOW HANDLERS`、`SHOW HANDLER`、`SHUTDOWN`、`BACKUP`、`KILL QUERY`、`KILL TRANSACTION`、`MOVE PARTITION BETWEEN SHARDS`、该切片里的 named collection 权限、`SET DEFINER`、`DISPLAYSECRETSINSHOWANDSELECT`、`TABLE ENGINE` 和 `CLUSTER`。
- `AI FUNCTIONS`，以及规范化名字至少有 3 个字符且以 `AI` 开头的单个标记权限，例如 `aiGenerate`。

只在全局 `*.*` 上拒绝：

- `ALL`、`ALL PRIVILEGES`、`CREATE`、`ALTER`、`DROP`、`TRUNCATE`、`OPTIMIZE`、`UNDROP TABLE`，以及 `globalDDLPrivilegeNames` 里的每个子权限，包括 `CREATE TEMPORARY TABLE` 和 `ALTER HANDLER`。

同一个 DDL 权限授在一个数据库上时，不会触发这次检查。授在一个数据库上的 `INSERT` 也不会。工具查询仍然设置 `readonly=1`。

当 `SELECT`、`ALL` 或 `ALL PRIVILEGES` 能读到查询文本或账号行时拒绝：

- `system.processes`
- `system.query_log`
- `system.query_thread_log`
- `system.query_views_log`
- `system.user_query_log`
- `system.query_cache`
- `system.asynchronous_inserts`
- `system.asynchronous_query_log`
- `system.users`

`*.*` 上的 `SELECT` 会命中上面每一张表。未加反引号、出现在名称末尾的 `*` 是前缀，因此 `system.proc*`、`system.user*`、`system.a*`、`sys*.*` 和 `s*.*` 都会命中。反引号里的 `*` 是字面名称。启动测试允许 `` GRANT SELECT ON system.`proc*` ``。比较区分大小写，因此在这次检查里 `system.Processes` 和 `SYSTEM.processes` 都不匹配 `system.processes`。`system.data*` 能匹配 `system.databases`，但不能匹配 `system.processes`，因此这种授权可以启动。`REVOKE SELECT ON system.processes` 只拿掉这一张精确表，`GRANT SELECT ON *.*` 仍然有效。

单独的 `SHOW` 不命中这条查询文本规则。`*.*` 上的 `GRANT SHOW DATABASES`、`SHOW TABLES` 和 `SHOW COLUMNS` 可以启动。列级的 `SELECT(query, user) ON system.processes` 会被拒绝。

启动还会从 `system.settings` 读取 `allow_experimental_ai_functions`。`is_obsolete` 为 false 且值为 true 时，拒绝该账号。过时的 true 值不会拒绝。未知设置不会拒绝。设置探测失败时，即使 `allow_unsafe_database_role` 为 true，启动仍会停止。启动测试记录了 ClickHouse 26.5.6.64、26.5.7.64 和 26.10.2.103 会把 `GRANT AI FUNCTIONS` 当成语法错误。服务器如果返回这段授权文本，单元测试仍会拒绝它。集成镜像是 `clickhouse/clickhouse-server:26.5.6.64`。只有当 `system.privileges` 含有该行时，脚本才会追加一次真实的 `AI FUNCTIONS` 授权。

`allow_unsafe_database_role` 默认为 false。只有无法创建更窄的账号，并且你控制这台 ClickHouse 时，才在 YAML 里设为 true，或导出 `MCP_ALLOW_UNSAFE_DATABASE_ROLE=true`。这个键没有标志。服务器会记录下面的警告，并只跳过这次权限拒绝：

```text
database service role is not permitted; allow_unsafe_database_role is set, so the startup role check is skipped
```

日志级别是 `warn`。HTTP 模式在默认的 `info` 级别下可以看到它。连接被拒绝、权限探测失败、AI 设置探测失败，以及角色展开超过上限，仍会停止启动。这个开关不会去掉 `readonly=1`。

`config check --connect` 不运行这次检查。`mcp` 和 `tools call` 会运行。

下面的语句要在 ClickHouse 上由管理员执行。它不是给 `clickhouse_query` 用的语句。

```sql
CREATE USER mcp_reader IDENTIFIED WITH no_password;
GRANT SELECT ON analytics.* TO mcp_reader;
GRANT SELECT ON system.databases TO mcp_reader;
GRANT SELECT ON system.tables TO mcp_reader;
GRANT SELECT ON system.columns TO mcp_reader;
```

内置的发现工具会读取这三张 system 表。查询工具需要读哪些库，就对那些库授予 `SELECT`。把 `analytics` 换成那个库。

## 命令行

不带参数的 `clickhouse-mcp-server` 会打印帮助，不会启动服务器。子命令是 `mcp`、`config`、`tools`（别名 `tool`）、`version` 和 `completion`。

```bash
./bin/clickhouse-mcp-server tools list --config config.yaml
./bin/clickhouse-mcp-server tools describe clickhouse_query --config config.yaml --json
./bin/clickhouse-mcp-server tools call clickhouse_list_databases --config config.yaml --params '{}'
./bin/clickhouse-mcp-server tools call clickhouse_query --config config.yaml \
  --params '{"query":"SELECT 1 AS value"}'
```

`tools describe` 接受别名 `schema`。`tools call` 把 `--params` 读成一个 JSON 对象。默认值是 `{}`。`--params-file -` 从 stdin 读取对象。`null` 会被拒绝，错误是 `parse tool params JSON: expected an object`。

`completion` 接受 `bash`、`zsh`、`fish`、`powershell` 和 `install`。

## 开发和测试

```bash
make format
make lint
make test
make build
make ci
```

`make ci` 运行 `lint`、`test` 和 `build`。`lint` 运行 `go vet`，并检查 `cmd`、`internal` 和 `pkg` 的 `gofmt`。安装了 `golangci-lint` 时它会运行该程序。GitHub Actions 的构建工作流使用 golangci-lint v2.12.2，运行 `go test -race ./...`，并运行 `make test-template-init`。`make ci` 不运行竞态检测，也不运行集成脚本。

`make test` 不依赖外部服务。`//go:build integration` 把 ClickHouse 测试排除在这条命令之外。`make test-integration` 需要 Docker 和 `curl`。除非你设置 `CLICKHOUSE_INTEGRATION_IMAGE`，否则它启动 `clickhouse/clickhouse-server:26.5.6.64`，最多等待 45 秒，并在退出时删除容器。它创建 `mcp_reader` 和 `wild_reader`，期望启动拒绝 `wild_reader`，并运行 HTTP 端到端测试、工具集集成测试和 `config check --connect`。构建工作流不会运行它。

`scripts/` 和 `templates/` 是保留的模板文件。它们不是 ClickHouse 服务器的运行时。`./scripts/init-template.sh` 用来初始化一个从模板创建的新仓库。这个仓库已经初始化。不要把它当作构建步骤来运行。`make test-template-init` 在临时副本里检查这个脚本。构建工作流会运行这项检查。

这个仓库的 issue 记录在 <https://github.com/futuretea/clickhouse-mcp-server/issues>。代码和注释保持英文。请让本文件和 [README.md](README.md) 保持一致。一次发布的平台名称写在 [`.github/ci/release-platforms.json`](.github/ci/release-platforms.json)。稳定标签 `vX.Y.Z` 会运行 npm 和 Docker 发布工作流。这些工作流创建的 GitHub Release 有说明，没有上传的二进制文件。发布 npm 需要 `NPM_TOKEN` 密钥。

## 排查

- `clickhouse.address is required`：设置 `host:port`，或设置 `MCP_CLICKHOUSE_ADDRESS`。
- `load config: clickhouse.address is invalid`：去掉 scheme、path、query 和 userinfo。
- `load config: configuration is invalid`：YAML 文件缺失，或不是有效配置。这条消息不会引用文件内容。
- `invalid masking config`：脱敏文档为空、包含未知内容，或已启用但没有规则。
- `[clickhouse_unsafe_service_role]`：账号命中[服务账号](#服务账号)里的规则。请使用只有 `SELECT` 的用户，或只在你控制的实例上设置 `allow_unsafe_database_role`。
- `[clickhouse_startup_failed] ClickHouse startup check failed:`：脱敏关闭，冒号后面是驱动文本。被拒绝的连接会包含 `connection refused`。用户名为空时，这段文本里的用户是 `default`。已配置的密码会被换成 `[redacted]`。
- `[clickhouse_startup_failed] ClickHouse startup check failed`：脱敏打开。没有驱动文本。
- `[clickhouse_query_failed]`、`[clickhouse_connect_failed]`、`[clickhouse_setting_failed]` 和 `[clickhouse_masking_failed]`：脱敏已打开，或结果无法完成脱敏。不会返回部分工具结果。
- `ClickHouse ping failed:`：`config check --connect` 无法 ping。这次检查不判断权限。
- `truncated: true`：在 SQL 里缩小结果，或有意提高 `max_rows`。
- `query must contain exactly one SQL statement`：只发送一条语句。
- `no tools registered`：工具或域过滤去掉了全部工具。
- `Could not resolve binary path`：操作系统和 CPU 不属于已发布的五个平台包。

## License

[MIT](LICENSE)
