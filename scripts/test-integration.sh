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
    clickhouse_exec "GRANT SELECT, SHOW ON *.* TO mcp_reader"
    MCP_CLICKHOUSE_ADDRESS="127.0.0.1:$port" \
      MCP_CLICKHOUSE_USERNAME="mcp_reader" \
      go test -count=1 -tags=integration ./cmd/mcp-server ./pkg/toolset/clickhouse ./internal/cmd
    exit 0
  fi
  sleep 1
done

printf '%s\n' 'error: temporary ClickHouse did not become ready within 45 seconds' >&2
docker logs "$container_name" --tail 80 >&2 || true
exit 1
