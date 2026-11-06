#!/usr/bin/env bash
# Boots a built image against a real Postgres and checks it serves. Unit tests cannot
# catch a broken Dockerfile, a missing runtime file or a migration that fails on start.
#
#   scripts/image-smoke.sh wamux:local
set -euo pipefail

image="${1:?usage: image-smoke.sh <image>}"
network="wamux-smoke-$$"
port="${SMOKE_PORT:-18080}"
base="http://127.0.0.1:${port}"

cleanup() {
	docker rm -f "${network}-app" "${network}-db" >/dev/null 2>&1 || true
	docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT

fail() {
	echo "FAIL: $1" >&2
	docker logs --tail 60 "${network}-app" >&2 || true
	exit 1
}

docker network create "$network" >/dev/null
docker run -d --name "${network}-db" --network "$network" \
	-e POSTGRES_USER=wamux -e POSTGRES_PASSWORD=wamux -e POSTGRES_DB=wamux_auth \
	postgres:18-alpine >/dev/null

for _ in $(seq 60); do
	docker exec "${network}-db" pg_isready -U wamux -d wamux_auth >/dev/null 2>&1 && break
	sleep 1
done
# The entrypoint restarts Postgres once after init, so make sure this one is final.
sleep 2
docker exec "${network}-db" createdb -U wamux wamux_users

db="postgresql://wamux:wamux@${network}-db:5432"
docker run -d --name "${network}-app" --network "$network" -p "127.0.0.1:${port}:8080" \
	-e SERVER_PORT=8080 \
	-e POSTGRES_AUTH_DB="${db}/wamux_auth?sslmode=disable" \
	-e POSTGRES_USERS_DB="${db}/wamux_users?sslmode=disable" \
	"$image" >/dev/null

for _ in $(seq 60); do
	curl -fsS --max-time 3 "$base/server/ok" >/dev/null 2>&1 && break
	[ "$(docker inspect -f '{{.State.Running}}' "${network}-app")" = "true" ] || fail "the container exited"
	sleep 1
done
curl -fsS --max-time 5 "$base/server/ok" >/dev/null || fail "/server/ok never answered"
echo "ok: /server/ok"

curl -fsS --max-time 5 "$base/manager/" | grep -qi '<div id="root"' || fail "the manager is not served"
echo "ok: /manager"

status="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$base/instance/all")"
[ "$status" = "401" ] || fail "/instance/all without a key answered $status"
api_key="$(docker exec "${network}-app" cat /app/data/global-api-key)"
curl -fsS --max-time 5 -H "apikey: $api_key" "$base/instance/all" >/dev/null || fail "/instance/all refused the generated key"
echo "ok: authentication with the generated key"

docker logs "${network}-app" 2>&1 | grep -qF "$api_key" && fail "the generated key reached the logs"
echo "ok: the key is not in the logs"

applied="$(docker exec "${network}-db" psql -U wamux -d wamux_users -Atc 'select count(*) from schema_migrations')"
expected="$(grep -cE '"[0-9]{4}_[a-z]' "$(dirname "$0")/../pkg/migrations/migrations.go")"
[ "${applied:-0}" -eq "$expected" ] || fail "$applied of $expected migrations were recorded"
echo "ok: $applied migrations applied"

for _ in $(seq 60); do
	[ "$(docker inspect -f '{{.State.Health.Status}}' "${network}-app")" = "healthy" ] && break
	sleep 1
done
[ "$(docker inspect -f '{{.State.Health.Status}}' "${network}-app")" = "healthy" ] || fail "the healthcheck never passed"
echo "ok: healthcheck"
