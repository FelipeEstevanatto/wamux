#!/usr/bin/env bash
# Boots a built image against a real Postgres and checks it serves. Unit tests cannot
# catch a broken Dockerfile, a missing runtime file or a migration that fails on start.
#
#   scripts/image-smoke.sh wamux:local
set -euo pipefail

image="${1:?usage: image-smoke.sh <image>}"
network="wamux-smoke-$$"
api_key="smoke-test-key"
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
	-e DATABASE_SAVE_MESSAGES=true \
	-e GLOBAL_API_KEY="$api_key" \
	-e CLIENT_NAME=smoke \
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
curl -fsS --max-time 5 -H "apikey: $api_key" "$base/instance/all" >/dev/null || fail "/instance/all refused the API key"
echo "ok: authentication"

applied="$(docker exec "${network}-db" psql -U wamux -d wamux_users -Atc 'select count(*) from schema_migrations')"
[ "${applied:-0}" -gt 0 ] || fail "no migrations were recorded"
echo "ok: $applied migrations applied"
