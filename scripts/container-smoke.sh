#!/usr/bin/env bash
set -euo pipefail
image=${1:?Usage: scripts/container-smoke.sh IMAGE EXPECTED_VERSION}
version=${2:?Expected version is required}
name="sublane-smoke-${RANDOM}-$$"
volume="${name}-data"
cleanup() {
  status=$?
  if [[ $status -ne 0 ]]; then docker logs "$name" >&2 || true; fi
  docker rm --force --volumes "$name" >/dev/null 2>&1 || true
  docker volume rm "$volume" >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM
wait_healthy() {
  for ((attempt=0; attempt<60; attempt++)); do
    if [[ $(docker inspect --format '{{.State.Health.Status}}' "$name") == healthy ]]; then return; fi
    if [[ $(docker inspect --format '{{.State.Running}}' "$name") != true ]]; then break; fi
    sleep 1
  done
  echo 'Container did not become healthy' >&2
  return 1
}
read_state() {
  docker exec "$name" wget -q -T 2 -O - http://127.0.0.1:8080/api/auth/state
}
docker volume create "$volume" >/dev/null
# All credentials/data are synthetic, and the container cannot contact providers or GitHub.
docker run --detach --name "$name" --network none \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=64m,mode=1777 \
  --cap-drop ALL --security-opt no-new-privileges:true \
  --health-interval 1s --health-start-period 10s \
  --mount "type=volume,source=$volume,target=/data" "$image" >/dev/null
wait_healthy
[[ $(docker exec "$name" sublane --version) == "$version" ]]
[[ $(docker exec "$name" id -u) != 0 ]]
read_state | jq -e '.initialized == false' >/dev/null
docker exec "$name" wget -q -T 5 -O - \
  --header='Content-Type: application/json' --header='Origin: http://127.0.0.1:8080' \
  --post-data='{"username":"container-admin","password":"synthetic-password","workspace_name":"Synthetic workspace"}' \
  http://127.0.0.1:8080/api/auth/setup | jq -e '.initialized == true' >/dev/null
[[ $(docker exec "$name" stat -c %a /data/credentials.key) == 600 ]]
# Root owns /app: probe as root so ordinary Unix permissions cannot mask a writable rootfs.
if docker exec --user 0 "$name" sh -c 'touch /app/readonly-test' >/dev/null 2>&1; then
  echo 'Runtime filesystem unexpectedly writable' >&2
  exit 1
fi
docker exec "$name" sublane backup --output /data/smoke.sublane-backup.tar.gz >/dev/null
docker exec "$name" sublane backup verify --input /data/smoke.sublane-backup.tar.gz >/dev/null
docker exec "$name" sublane restore --input /data/smoke.sublane-backup.tar.gz --data-dir /data/restored >/dev/null
docker restart --time 20 "$name" >/dev/null
wait_healthy
read_state | jq -e '.initialized == true' >/dev/null
docker exec "$name" test -f /data/restored/credentials.key
printf 'Container smoke passed: %s (%s)\n' "$image" "$version"
