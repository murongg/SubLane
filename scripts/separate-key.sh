#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  cat <<'HELP'
Separate an existing SubLane Compose credential key, or roll back that migration.

Usage: sudo bash separate-key.sh --dir DIRECTORY --key-dir NEW_ABSOLUTE_DIRECTORY
                                [--wait-timeout SECONDS]
       sudo bash separate-key.sh --dir DIRECTORY --rollback
                                [--wait-timeout SECONDS]

Requires local Linux rootful Docker, Compose, Bash, jq and GNU coreutils.
The existing running service must be named sublane and mount its data directory
at a mount root. Block environment mappings and volume lists are supported. Existing
key destinations, remote/rootless/userns Docker and ambiguous layouts are refused.

The original Compose project, image, data mount and configuration are preserved.
Encrypted backup/verification uses a temporary random password and private scratch
outside the data directory; no backup password is accepted or retained. This is a
credential check, not a retained recovery backup. Export and verify your own
password-encrypted backup first. Do not run other deployment operations concurrently.

Failures after switching begins attempt automatic rollback. Recovery metadata and
original configuration remain in DIRECTORY/.key-migration; --rollback also works
after a successful migration. If rollback fails, recovery key material is retained.
After a forced kill, confirm no update/migration is running before removing a stale
DIRECTORY/.update-lock and using --rollback. Rollback never downgrades an image or DB.
HELP
}
fail() { printf 'Error: %s\n' "$*" >&2; exit 1; }
sha() { local value; value=$(sha256sum -- "$1") || return 1; printf '%s\n' "${value%% *}"; }
inside() { [[ $2 == / && $1 == /* || $1 == "$2" || $1 == "$2/"* ]]; }
model_digest() { local value; value=$(jq -S . | sha256sum) || return 1; printf '%s\n' "${value%% *}"; }
private_file() {
  [[ -f $1 && ! -L $1 ]] || return 1
  local mode; mode=$(stat -c %a "$1") || return 1
  (( (8#$mode & 0077) == 0 ))
}
key_matches() { private_file "$1" && [[ $(stat -c %s "$1") == 32 && $(sha "$1") == "$key_hash" ]]; }
engine() { timeout 120 docker "$@" 2>>"$log"; }
compose_file() {
  local file=$1; shift
  engine compose --project-name "$project" --project-directory "$directory" \
    --env-file "$env_file" -f "$file" "$@"
}
compose() { compose_file "$compose_path" "$@"; }
env_hash() {
  if [[ $env_file == /dev/null ]]; then
    [[ ! -e $directory/.env && ! -L $directory/.env ]] || return 1
    printf 'absent\n'
  else
    [[ -f $env_file && ! -L $env_file ]] || return 1
    sha "$env_file"
  fi
}
phase() {
  jq --arg phase "$1" '.phase=$phase' "$state/metadata.json" > "$state/metadata.tmp" || return 1
  mv -- "$state/metadata.tmp" "$state/metadata.json" || return 1
  sync -f "$state"
}
unchanged() {
  [[ $(env_hash) == "$saved_env_hash" && $(stat -c '%d:%i' "$data_source") == "$data_identity" ]] || return 1
  [[ $(engine image inspect --format '{{.Id}}' "$image") == "$image_id" ]] || return 1
  local current; current=$(sha "$compose_path") || return 1
  [[ $current == "$original_hash" || $current == "$new_hash" ]]
}
container() {
  # Compose one-off verification containers belong to the project but are not the service.
  local ids; ids=$(engine ps --all --quiet --filter "label=com.docker.compose.project=$project" \
    --filter 'label=com.docker.compose.service=sublane' --filter 'label=com.docker.compose.oneoff=False' \
    --filter 'label=com.docker.compose.config-hash') || return 1
  if [[ -z $ids ]]; then printf '[]\n'; return 0; fi
  [[ $ids =~ ^[a-zA-Z0-9][a-zA-Z0-9_-]*$ ]] || return 1
  engine inspect "$ids"
}
owned_scope() {
  jq -e --arg image "$image_id" --arg project "$project" --arg data "$data_dir" \
    --arg source "$data_source" --arg keys "$key_dir" --arg dir "$directory" --arg compose "$compose_path" '
    if length==0 then true else length==1 and
    .[0].Image==$image and .[0].Config.Labels["com.docker.compose.project"]==$project and
    .[0].Config.Labels["com.docker.compose.project.working_dir"]==$dir and
    .[0].Config.Labels["com.docker.compose.project.config_files"]==$compose and
    ([.[0].Config.Env[]|select(startswith("SUBLANE_DATA_DIR="))|ltrimstr("SUBLANE_DATA_DIR=")]|first//"/data")==$data and
    ([.[0].Mounts[]|select(.Destination==$data and .Source==$source and .RW==true)]|length)==1 and
    ([.[0].Mounts[]|select(.Destination==($data+"/credentials.key"))]|length)==0 and
    (([.[0].Config.Env[]|select(startswith("SUBLANE_CREDENTIALS_KEY_FILE="))|ltrimstr("SUBLANE_CREDENTIALS_KEY_FILE=")]|first//"") as $key |
      $key=="" or ($key=="/run/sublane-keys/credentials.key" and
        ([.[0].Mounts[]|select(.Destination==$key and .Source==($keys+"/credentials.key") and .RW==false)]|length)==1)) end
  ' "$state/container.json" >/dev/null || return 1
}
ready() {
  local expected=$1
  unchanged || return 1
  container > "$state/container.json" && owned_scope || return 1
  jq -e --arg expected "$expected" '
    length==1 and .[0].State.Running==true and .[0].State.Health.Status=="healthy" and
    ([.[0].Config.Env[]|select(startswith("SUBLANE_CREDENTIALS_KEY_FILE="))|ltrimstr("SUBLANE_CREDENTIALS_KEY_FILE=")]|first//"")==$expected
  ' "$state/container.json" >/dev/null || return 1
  local actual; actual=$(compose exec -T sublane sha256sum "${expected:-$data_dir/credentials.key}") || return 1
  [[ ${actual%% *} == "$key_hash" ]]
}
# Expansion in this function's quoted program belongs to the container shell.
# shellcheck disable=SC2016
deep_check() {
  install -d -m 0700 -o "$service_uid" -g "$service_gid" "$state/check" || return 1
  # A bind outside /data avoids Docker's small /tmp and keeps decrypted verification copies out of data backups.
  timeout 960 docker compose --project-name "$project" --project-directory "$directory" \
    --env-file "$env_file" -f "$compose_path" run --rm --no-deps -T --pull never \
    --volume "$state/check:/run/sublane-check" --env SUBLANE_CHECK_DIR=/run/sublane-check \
    --entrypoint /bin/sh sublane -ec '
      umask 077
      scratch=$(mktemp -d "$SUBLANE_CHECK_DIR/check.XXXXXXXX")
      trap '\''rm -rf -- "$scratch"'\'' EXIT
      mkdir "$scratch/tmp"
      export TMPDIR="$scratch/tmp"
      head -c 32 /dev/urandom | base64 > "$scratch/password"
      sublane backup --output "$scratch/verify.age" --password-stdin < "$scratch/password" >/dev/null
      test "$(head -n 1 "$scratch/verify.age")" = age-encryption.org/v1
      sublane backup verify --input "$scratch/verify.age" --password-stdin < "$scratch/password" >/dev/null
    ' >/dev/null 2>>"$log" || return 1
  rm -rf -- "$state/check"
}
start() {
  timeout "$((wait_timeout+60))" docker compose --project-name "$project" --project-directory "$directory" \
    --env-file "$env_file" -f "$compose_path" up --detach --wait --wait-timeout "$wait_timeout" \
    --no-build --pull never --no-deps --force-recreate sublane >/dev/null 2>>"$log"
}
restore_config() {
  install -m "$original_mode" -o "$original_uid" -g "$original_gid" \
    "$state/original.compose" "$state/restoring.compose" || return 1
  mv -- "$state/restoring.compose" "$compose_path"
}
rollback() {
  # Explicit checks are required: Bash disables errexit inside functions used by an `if` error handler.
  unchanged && [[ $(sha "$state/original.compose") == "$original_hash" ]] || return 1
  # A new shell can change Compose interpolation without changing either saved file.
  local model; model=$(compose_file "$state/original.compose" config --format json | model_digest) || return 1
  [[ $model == "$original_model_hash" ]] || return 1
  # Refuse to stop a replacement instance; an absent container is recoverable after a failed recreation.
  container > "$state/container.json" && owned_scope || return 1
  local source="$state/source.key" old="$data_source/credentials.key"
  if ! key_matches "$source"; then source="$key_dir/credentials.key"; fi
  if [[ -e $old || -L $old ]]; then key_matches "$old" || return 1
  else key_matches "$source" || return 1; fi
  if [[ $(sha "$compose_path") == "$original_hash" ]] && ready ''; then
    deep_check || return 1
  else
    compose stop sublane >/dev/null || return 1
    if [[ ! -e $old ]]; then
      install -m 0600 -o "$service_uid" -g "$service_gid" "$source" "$old" || return 1
      sync -f "$old" || return 1
    fi
    key_matches "$old" || return 1
    if [[ $(sha "$compose_path") != "$original_hash" ]]; then restore_config || return 1; fi
    start && ready '' && deep_check || return 1
  fi
  phase rolled_back || return 1
  rm -f -- "$state/source.key" || return 1
  printf 'Rollback verified. Original Compose configuration and credential key are active.\n'
}
finish() {
  local status=$?
  trap - EXIT INT TERM HUP
  if ((status != 0)) && [[ $switching == true ]]; then
    printf 'Migration failed; attempting rollback.\n' >&2
    if rollback; then switching=false
    else
      recovery_failed=true
      printf 'Rollback could not be verified. Keep %s and %s; inspect the private operation.log and rerun with --rollback.\n' "$state" "$key_dir" >&2
    fi
  fi
  if [[ $state_ready == true ]]; then
    rm -rf -- "$state/check" || status=1
    if [[ $recovery_failed == false ]]; then rm -f -- "$state/source.key" || status=1; fi
  fi
  if [[ -n $lock ]]; then rmdir -- "$lock" || status=1; fi
  exit "$status"
}

directory='' key_dir='' rollback_mode=false wait_timeout=90
state='' lock='' log=/dev/null switching=false recovery_failed=false state_ready=false
while (($#)); do
  case "$1" in
    --dir|--key-dir|--wait-timeout)
      (($#>=2)) || fail "Missing value for $1."
      case "$1" in --dir) directory=$2 ;; --key-dir) key_dir=$2 ;; --wait-timeout) wait_timeout=$2 ;; esac
      shift 2 ;;
    --rollback) rollback_mode=true; shift ;;
    --help|-h) usage; exit 0 ;;
    *) fail 'Unknown option; use --help.' ;;
  esac
done
[[ -n $directory && $wait_timeout =~ ^[1-9][0-9]{0,3}$ && $wait_timeout -le 1800 ]] || fail 'Specify --dir and a wait timeout from 1 to 1800 seconds.'
[[ $rollback_mode == true && -z $key_dir || $rollback_mode == false && -n $key_dir ]] || fail 'Use --key-dir for migration, or --rollback without --key-dir.'
[[ $(uname -s) == Linux && $(id -u) == 0 ]] || fail 'Run as root on the local Linux Docker host.'
for tool in docker jq timeout realpath stat install sha256sum awk cmp sync; do command -v "$tool" >/dev/null || fail "Required command unavailable: $tool."; done
[[ -d $directory && ! -L $directory ]] || fail 'Deployment directory must be an existing real directory.'
directory=$(realpath -e -- "$directory")
compose_path="$directory/docker.compose.yaml"
[[ -f $compose_path && ! -L $compose_path ]] || fail 'Expected a regular docker.compose.yaml file.'
env_file=/dev/null
if [[ -e $directory/.env || -L $directory/.env ]]; then
  [[ -f $directory/.env && ! -L $directory/.env ]] || fail 'The deployment .env must be a regular file.'
  env_file="$directory/.env"
fi
endpoint=${DOCKER_HOST:-$(docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null)}
[[ $endpoint == unix://* ]] || fail 'Only a local Unix-socket Docker daemon is supported.'
daemon=$(timeout 120 docker info --format '{{json .}}' 2>/dev/null) || fail 'Docker is unavailable.'
printf '%s' "$daemon" | jq -e '.OSType=="linux" and ([.SecurityOptions[]? | select(test("rootless|userns|selinux"))]|length)==0' >/dev/null \
  || fail 'Only local Linux rootful Docker without rootless/userns/SELinux is supported.'

state="$directory/.key-migration"
lock="$directory/.update-lock"
mkdir -m 0700 -- "$lock" || { lock=''; fail 'Another deployment operation is running, or its lock needs inspection.'; }
trap finish EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP
if [[ $rollback_mode == false ]]; then
  [[ ! -e $state && ! -L $state ]] || fail 'Recovery state already exists; inspect it and use --rollback.'
  [[ $key_dir =~ ^/[a-zA-Z0-9._/\ -]+$ && ! -e $key_dir && ! -L $key_dir ]] || fail 'Use a new absolute key directory (letters, numbers, spaces, /._-).'
  key_dir=$(realpath -m -- "$key_dir")
  [[ -d $(dirname -- "$key_dir") ]] || fail 'The key directory parent must already exist.'
  configuration=$(timeout 120 docker compose --project-directory "$directory" --env-file "$env_file" -f "$compose_path" config --format json 2>/dev/null) || fail 'Could not parse Compose configuration.'
  original_model_hash=$(printf '%s' "$configuration" | model_digest)
  daemon_id=$(printf '%s' "$daemon" | jq -er .ID)
  project=$(printf '%s' "$configuration" | jq -er '.name')
  image=$(printf '%s' "$configuration" | jq -er '.services.sublane.image')
  data_dir=$(printf '%s' "$configuration" | jq -er '.services.sublane.environment.SUBLANE_DATA_DIR//"/data"')
  [[ $data_dir == /* && $data_dir != / && $data_dir != *$'\n'* ]] || fail 'Unsupported data directory.'
  [[ $(printf '%s' "$configuration" | jq -r '.services.sublane.environment.SUBLANE_CREDENTIALS_KEY_FILE//""') == '' ]] || fail 'The service already has a separate key configured.'
  inspected=$(container) || fail 'Expected exactly one existing service container.'
  printf '%s' "$inspected" | jq -e --arg dir "$directory" --arg project "$project" --arg compose "$compose_path" --arg data "$data_dir" '
    length==1 and .[0].State.Running==true and .[0].State.Health.Status=="healthy" and
    .[0].Config.Labels["com.docker.compose.project"]==$project and
    .[0].Config.Labels["com.docker.compose.project.working_dir"]==$dir and
    .[0].Config.Labels["com.docker.compose.project.config_files"]==$compose and
    ([.[0].Config.Env[]|select(startswith("SUBLANE_DATA_DIR="))|ltrimstr("SUBLANE_DATA_DIR=")]|first//"/data")==$data and
    ([.[0].Config.Env[]|select(startswith("SUBLANE_CREDENTIALS_KEY_FILE="))|ltrimstr("SUBLANE_CREDENTIALS_KEY_FILE=")]|first//"")=="" and
    ([.[0].Mounts[]|select(.Destination==($data+"/credentials.key"))]|length)==0
  ' >/dev/null || fail 'Use the healthy existing project with matching data/key settings and one configuration file.'
  [[ $(printf '%s' "$configuration" | jq --arg data "$data_dir" '[.services.sublane.volumes[]?|select(.target==($data+"/credentials.key"))]|length') == 0 ]] || fail 'The original key must live directly in the data mount.'
  data_source=$(printf '%s' "$inspected" | jq -er --arg data "$data_dir" '[.[0].Mounts[]|select(.Destination==$data and .RW==true)]|if length==1 then .[0].Source else error("ambiguous data mount") end') || fail 'The data directory must be a writable mount root.'
  data_source=$(realpath -e -- "$data_source")
  if inside "$key_dir" "$data_source" || inside "$state" "$data_source" || inside "$key_dir" "$state" || inside "$directory" "$key_dir"; then
    fail 'Key and recovery directories must be outside the data directory and each other.'
  fi
  configured_mount=$(printf '%s' "$configuration" | jq -ce --arg data "$data_dir" '[.services.sublane.volumes[]?|select(.target==$data)]|if length==1 and (.[0].read_only//false)==false then .[0] else error("ambiguous data mount") end') || fail 'Expected one writable configured data mount.'
  mounted_type=$(printf '%s' "$inspected" | jq -r --arg data "$data_dir" '.[0].Mounts[]|select(.Destination==$data)|.Type')
  [[ $(printf '%s' "$configured_mount" | jq -r .type) == "$mounted_type" ]] || fail 'Configured data mount differs from the running container.'
  if [[ $mounted_type == bind ]]; then
    configured_source=$(printf '%s' "$configured_mount" | jq -er .source)
    [[ $(realpath -e -- "$configured_source") == "$data_source" ]] || fail 'Configured data source differs from the running container.'
  elif [[ $mounted_type == volume ]]; then
    volume_name=$(printf '%s' "$configured_mount" | jq -er .source)
    configured_name=$(printf '%s' "$configuration" | jq -er --arg volume "$volume_name" '.volumes[$volume].name')
    mounted_name=$(printf '%s' "$inspected" | jq -er --arg data "$data_dir" '.[0].Mounts[]|select(.Destination==$data)|.Name')
    [[ $configured_name == "$mounted_name" ]] || fail 'Configured volume differs from the running container.'
  else
    fail 'Only bind mounts and local named data volumes are supported.'
  fi
  [[ $(printf '%s' "$configuration" | jq '[.services.sublane.volumes[]?|select(.target=="/run/sublane-keys" or .target=="/run/sublane-keys/credentials.key")]|length') == 0 ]] || fail 'The separate-key mount target is already used.'
  if ! { private_file "$data_source/credentials.key" && [[ $(stat -c %s "$data_source/credentials.key") == 32 ]]; }; then
    fail 'The original credential key must be a private regular 32-byte file.'
  fi
  service_uid=$(compose exec -T sublane id -u); service_gid=$(compose exec -T sublane id -g)
  [[ $service_uid =~ ^[0-9]+$ && $service_gid =~ ^[0-9]+$ ]] || fail 'Could not resolve the service user.'
  help=$(compose exec -T sublane sublane --help)
  [[ $help == *--password-stdin* && $help == *--key-file* ]] || fail 'Upgrade to SubLane v0.12.0 or later first.'
  image_id=$(printf '%s' "$inspected" | jq -er '.[0].Image')
  original_hash=$(sha "$compose_path"); saved_env_hash=$(env_hash)
  key_hash=$(sha "$data_source/credentials.key"); data_identity=$(stat -c '%d:%i' "$data_source")
  original_uid=$(stat -c %u "$compose_path"); original_gid=$(stat -c %g "$compose_path"); original_mode=$(stat -c %a "$compose_path")
else
  [[ -d $state && ! -L $state && $(stat -c %u "$state") == 0 && $(stat -c %a "$state") == 700 ]] || fail 'Expected private root-owned recovery state.'
  if ! { private_file "$state/metadata.json" && private_file "$state/original.compose"; }; then
    fail 'Recovery files must be private regular files.'
  fi
  jq -e '.version==1' "$state/metadata.json" >/dev/null || fail 'Unsupported recovery metadata version.'
  for field in project image image_id daemon_id data_dir data_source key_dir key_hash data_identity original_hash original_model_hash new_hash saved_env_hash service_uid service_gid original_uid original_gid original_mode; do
    value=$(jq -er --arg field "$field" '.[$field]|strings' "$state/metadata.json") || fail 'Invalid recovery metadata.'
    printf -v "$field" '%s' "$value"
  done
  [[ $(jq -r '.directory' "$state/metadata.json") == "$directory" ]] || fail 'Recovery state belongs to another deployment.'
  [[ $(printf '%s' "$daemon" | jq -er .ID) == "$daemon_id" ]] || fail 'Recovery state belongs to another Docker daemon.'
fi
if [[ $rollback_mode == true ]]; then
  state_ready=true
  log="$state/operation.log"
  rollback || { recovery_failed=true; fail 'Rollback failed; recovery key material was retained. Inspect the private operation.log.'; }
  exit 0
fi
mkdir -m 0700 -- "$state"
state_ready=true
log="$state/operation.log"
cp -- "$compose_path" "$state/original.compose"
cp -- "$data_source/credentials.key" "$state/source.key"
chmod 0600 "$state/original.compose" "$state/source.key"
printf '%s\n' "$configuration" > "$state/before.json"
new_hash=''
jq -n --arg directory "$directory" --arg project "$project" --arg image "$image" --arg image_id "$image_id" --arg daemon_id "$daemon_id" \
  --arg data_dir "$data_dir" --arg data_source "$data_source" --arg key_dir "$key_dir" --arg key_hash "$key_hash" \
  --arg data_identity "$data_identity" --arg original_hash "$original_hash" --arg original_model_hash "$original_model_hash" --arg saved_env_hash "$saved_env_hash" \
  --arg service_uid "$service_uid" --arg service_gid "$service_gid" --arg original_uid "$original_uid" --arg original_gid "$original_gid" \
  --arg original_mode "$original_mode" '$ARGS.named+{version:1,phase:"prepared",new_hash:""}' > "$state/metadata.json"
# Restrict text edits to the incumbent mapping layout; Compose below proves no other semantic setting changed.
awk -v source="$key_dir" '
  /^  sublane:[[:space:]]*$/ {service=1; print; next}
  service && /^  [^ ]/ {service=0}
  service && /^    environment:[[:space:]]*$/ {environments++; print; print "      SUBLANE_CREDENTIALS_KEY_FILE: /run/sublane-keys/credentials.key"; next}
  service && /^      SUBLANE_CREDENTIALS_KEY_FILE:/ {next}
  service && /^    volumes:[[:space:]]*$/ {volumes++; print; print "      - type: bind\n        source: \"" source "/credentials.key\"\n        target: /run/sublane-keys/credentials.key\n        read_only: true\n        bind:\n          create_host_path: false"; next}
  {print}
  END {if (environments!=1 || volumes!=1) exit 1}
' "$state/original.compose" > "$state/candidate.compose" || fail 'Unsupported Compose layout; use a block environment mapping and volume list.'
compose_file "$state/candidate.compose" config --format json > "$state/after.json" || fail 'Candidate configuration is invalid.'
# Compose omits explicit false values from its canonical JSON.
jq -e --arg source "$key_dir" '
  .services.sublane.environment.SUBLANE_CREDENTIALS_KEY_FILE=="/run/sublane-keys/credentials.key" and
  ([.services.sublane.volumes[]|select(.target=="/run/sublane-keys/credentials.key" and .source==($source+"/credentials.key") and .type=="bind" and .read_only==true and .bind.create_host_path!=true)]|length)==1
' "$state/after.json" >/dev/null || fail 'Candidate key mount is invalid.'
for side in before after; do
  jq -S 'del(.services.sublane.environment.SUBLANE_CREDENTIALS_KEY_FILE)|.services.sublane.volumes|=map(select(.target!="/run/sublane-keys/credentials.key"))' \
    "$state/$side.json" > "$state/$side.normalized"
done
cmp -s "$state/before.normalized" "$state/after.normalized" || fail 'Candidate configuration changed unrelated settings.'
new_hash=$(sha "$state/candidate.compose")
jq --arg hash "$new_hash" '.new_hash=$hash' "$state/metadata.json" > "$state/metadata.tmp"
mv -- "$state/metadata.tmp" "$state/metadata.json"
if ! { ready '' && deep_check; }; then fail 'Original instance credential verification failed before migration.'; fi
[[ ! -e $key_dir && ! -L $key_dir ]] || fail 'Key destination appeared during preflight.'
# Claim it exclusively; another deployment must never have its key overwritten.
# Binding only the file keeps the root-private parent inaccessible to host processes sharing the container UID.
mkdir -m 0700 -- "$key_dir"
install -m 0600 -o "$service_uid" -g "$service_gid" "$state/source.key" "$key_dir/credentials.key"
if ! { key_matches "$key_dir/credentials.key" && cmp -s "$state/source.key" "$key_dir/credentials.key"; }; then
  fail 'External key copy validation failed.'
fi
sync -f "$key_dir/credentials.key"
if ! { ready '' && [[ $(sha "$compose_path") == "$original_hash" ]]; }; then
  fail 'Deployment changed during preflight.'
fi
switching=true
compose stop sublane >/dev/null
install -m "$original_mode" -o "$original_uid" -g "$original_gid" "$state/candidate.compose" "$state/applying.compose"
mv -- "$state/applying.compose" "$compose_path"
phase applied
if ! { start && ready /run/sublane-keys/credentials.key && deep_check; }; then fail 'Separated instance verification failed.'; fi
if ! { ready /run/sublane-keys/credentials.key && key_matches "$data_source/credentials.key"; }; then fail 'Original key or deployment changed; refusing deletion.'; fi
# Persist intent before unlinking: rollback must also cover a kill between deletion and the next startup.
phase removing_old
rm -- "$data_source/credentials.key"
sync -f "$data_source"
if ! { start && ready /run/sublane-keys/credentials.key && deep_check; }; then fail 'Post-deletion instance verification failed.'; fi
rm -f -- "$state/source.key"
rm -f -- "$state/before.json" "$state/after.json" "$state/before.normalized" "$state/after.normalized" "$state/container.json" "$state/candidate.compose"
phase complete
switching=false
printf 'Key separation verified. The old data-directory key was removed.\nRecovery configuration: %s\nRollback: sudo bash separate-key.sh --dir %q --rollback\n' "$state" "$directory"
