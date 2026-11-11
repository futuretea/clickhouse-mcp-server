#!/usr/bin/env bash

set -euo pipefail

readonly container_name="clickhouse-mcp-integration-$$"
readonly image="${CLICKHOUSE_INTEGRATION_IMAGE:-clickhouse/clickhouse-server:26.5.6.64}"

command -v docker >/dev/null || {
  printf '%s\n' 'error: Docker is required for make test-integration' >&2
  exit 1
}
command -v curl >/dev/null || {
  printf '%s\n' 'error: curl is required for make test-integration' >&2
  exit 1
}

cleanup() {
  docker stop "$container_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

clickhouse_exec() {
  local sql="$1"
  local response_file
  response_file=$(mktemp)
  local code
  code=$(curl --silent --show-error --output "$response_file" --write-out '%{http_code}' "http://127.0.0.1:${port}/" --data-binary "$sql") || {
    rm -f "$response_file"
    printf '%s\n' 'error: ClickHouse privilege setup request failed' >&2
    return 1
  }
  if [ "$code" != "200" ]; then
    printf '%s\n' "error: ClickHouse privilege setup failed (${code})" >&2
    cat "$response_file" >&2 || true
    rm -f "$response_file"
    return 1
  fi
  rm -f "$response_file"
}

clickhouse_query() {
  local sql="$1"
  local response_file
  response_file=$(mktemp)
  local code
  code=$(curl --silent --show-error --output "$response_file" --write-out '%{http_code}' "http://127.0.0.1:${port}/" --data-binary "$sql") || {
    rm -f "$response_file"
    return 1
  }
  if [ "$code" != "200" ]; then
    rm -f "$response_file"
    return 1
  fi
  cat "$response_file"
  rm -f "$response_file"
}

docker run --detach --rm --name "$container_name" \
  -e CLICKHOUSE_SKIP_USER_SETUP=1 \
  -p 127.0.0.1::8123 \
  "$image" >/dev/null

port=$(docker port "$container_name" 8123/tcp | awk -F: 'NR == 1 { print $NF }')
if [ -z "$port" ]; then
  printf '%s\n' 'error: could not determine the temporary ClickHouse HTTP port' >&2
  exit 1
fi

for _ in $(seq 1 45); do
  if curl --fail --silent --show-error "http://127.0.0.1:$port/ping" >/dev/null; then
    clickhouse_exec "CREATE USER IF NOT EXISTS mcp_reader IDENTIFIED WITH no_password"
    clickhouse_exec "GRANT SELECT ON system.databases TO mcp_reader"
    clickhouse_exec "GRANT SELECT ON system.tables TO mcp_reader"
    clickhouse_exec "GRANT SELECT ON system.columns TO mcp_reader"
    clickhouse_exec "CREATE USER IF NOT EXISTS wild_reader IDENTIFIED WITH no_password"
    clickhouse_exec "GRANT SELECT ON system.proc* TO wild_reader"
    MCP_CLICKHOUSE_ADDRESS="127.0.0.1:$port" \
      MCP_CLICKHOUSE_USERNAME="mcp_reader" \
      go test -count=1 -tags=integration ./cmd/mcp-server ./pkg/toolset/clickhouse ./internal/cmd
    MCP_CLICKHOUSE_ADDRESS="127.0.0.1:$port" \
      MCP_CLICKHOUSE_USERNAME="wild_reader" \
      MCP_CLICKHOUSE_WILDCARD_PROBE="1" \
      go test -count=1 -tags=integration -run TestIntegrationWildcardProcessGrantRejected ./pkg/toolset/clickhouse
    # ClickHouse 26.5 has no AI FUNCTIONS privilege row. Newer servers do.
    # The pinned default image skips this probe. A server that lists the
    # privilege must fail startup for an account that holds it.
    ai_privileges="$(clickhouse_query "SELECT count() FROM system.privileges WHERE toString(privilege) = 'AI FUNCTIONS' FORMAT TSV" || true)"
    ai_privileges="${ai_privileges//$'\n'/}"
    if [ "$ai_privileges" = "0" ] || [ -z "$ai_privileges" ]; then
      printf '%s\n' 'AI FUNCTIONS privilege is absent; unit tests cover that grant text'
    else
      clickhouse_exec "CREATE USER IF NOT EXISTS ai_probe IDENTIFIED WITH no_password"
      clickhouse_exec "GRANT SELECT ON system.databases TO ai_probe"
      clickhouse_exec "GRANT SELECT ON system.tables TO ai_probe"
      clickhouse_exec "GRANT SELECT ON system.columns TO ai_probe"
      clickhouse_exec "GRANT AI FUNCTIONS ON *.* TO ai_probe"
      MCP_CLICKHOUSE_ADDRESS="127.0.0.1:$port" \
        MCP_CLICKHOUSE_USERNAME="ai_probe" \
        MCP_CLICKHOUSE_AI_PROBE="1" \
        go test -count=1 -tags=integration -run TestIntegrationAIFunctionPrivilegeRejected ./pkg/toolset/clickhouse
    fi
    exit 0
  fi
  sleep 1
done

printf '%s\n' 'error: temporary ClickHouse did not become ready within 45 seconds' >&2
docker logs "$container_name" --tail 80 >&2 || true
exit 1
