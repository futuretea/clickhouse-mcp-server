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
    MCP_CLICKHOUSE_ADDRESS="127.0.0.1:$port" \
      go test -count=1 -tags=integration ./cmd/mcp-server ./pkg/toolset/clickhouse ./internal/cmd
    exit 0
  fi
  sleep 1
done

printf '%s\n' 'error: temporary ClickHouse did not become ready within 45 seconds' >&2
docker logs "$container_name" --tail 80 >&2 || true
exit 1
