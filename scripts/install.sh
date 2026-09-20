#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'HELP'
Install SubLane from a published Docker image.

Usage: bash install.sh [--version VERSION] [--dir DIRECTORY] [--port PORT]

  --version VERSION  Published version, with or without v (default: latest stable,
                     or newest published prerelease when no stable release exists)
  --dir DIRECTORY    New installation directory (default: ./sublane)
  --port PORT        Host port on 127.0.0.1 (default: 8080)
  --help             Show this help

Requires Bash, curl, Docker with Compose, and sha256sum or shasum.
Automatic version selection also requires jq.
Existing directories are never overwritten. Use the deployment guide for upgrades.
HELP
}

cleanup() {
  if [[ -n ${install_stage:-} ]]; then rm -rf -- "$install_stage"; fi
}

compose() {
  local directory=$1
  shift
  # Environment interpolation must not select another image, port or existing project.
  SUBLANE_IMAGE="$install_image" SUBLANE_BIND_ADDRESS=127.0.0.1 \
    SUBLANE_PORT="$install_port" SUBLANE_PUBLIC_URL='' SUBLANE_LOG_LEVEL=info \
    docker compose --project-name "$install_project" --project-directory "$directory" \
    --env-file "$directory/.env" -f "$directory/docker.compose.yaml" "$@" </dev/null
}

download() {
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --connect-timeout 10 --max-time 120 --retry 2 \
    --output "$install_stage/$1" "$install_release/$1" || fail "Could not download $1 for v$install_version."
}

validate_version() {
  local version_pattern='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+\.)*[0-9A-Za-z-]+)?$'
  [[ ${#install_version} -lt 100 && $install_version =~ $version_pattern ]] || fail 'Use a canonical version such as 1.2.3 or 1.2.3-rc.1.'
  if [[ $install_version == *-* ]]; then
    local part
    local -a prerelease_parts
    IFS=. read -r -a prerelease_parts <<< "${install_version#*-}"
    for part in "${prerelease_parts[@]}"; do
      [[ ! $part =~ ^0[0-9]+$ ]] || fail 'Numeric prerelease identifiers cannot have leading zeros.'
    done
  fi
}

release_metadata() {
  install_api_status=$(curl --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --connect-timeout 10 --max-time 120 --retry 2 --header 'Accept: application/vnd.github+json' \
    --output "$install_stage/releases.json" --write-out '%{http_code}' \
    "https://api.github.com/repos/murongg/SubLane/$1") || fail 'Could not fetch GitHub releases. Retry or pass --version explicitly.'
}

resolve_version() {
  release_metadata releases/latest
  if [[ $install_api_status == 200 ]]; then
    install_version=$(jq -er 'select(.draft == false and .prerelease == false) | .tag_name | select(type == "string")' \
      "$install_stage/releases.json") || fail 'Invalid latest release metadata.'
    install_version=${install_version#v}
    return
  fi
  # Only absence of a stable release permits fallback; rate limits and outages must not change channels.
  [[ $install_api_status == 404 ]] || fail "GitHub latest release request failed (HTTP $install_api_status). Retry or pass --version explicitly."

  local page count candidate published tag newest_date='' newest_tag=''
  for ((page=1; page<=20; page++)); do
    release_metadata "releases?per_page=100&page=$page"
    [[ $install_api_status == 200 ]] || fail "GitHub releases request failed (HTTP $install_api_status)."
    jq -e 'type == "array"' "$install_stage/releases.json" >/dev/null || fail 'Invalid release list.'
    count=$(jq 'length' "$install_stage/releases.json")
    candidate=$(jq -r '[.[] | select(.draft == false and .prerelease == true and (.tag_name | type) == "string" and (.published_at | type) == "string")]
      | max_by(.published_at) | if . == null then empty else [.published_at, .tag_name] | @tsv end' \
      "$install_stage/releases.json")
    if [[ -n $candidate ]]; then
      IFS=$'\t' read -r published tag <<< "$candidate"
      if [[ $published > $newest_date ]]; then
        newest_date=$published
        newest_tag=$tag
      fi
    fi
    if [[ $count -lt 100 ]]; then
      [[ -n $newest_tag ]] || fail 'No published stable or prerelease version is available.'
      install_version=${newest_tag#v}
      return
    fi
  done
  fail 'Release scan limit reached; pass --version explicitly.'
}

main() {
  install_version=''
  install_dir=$PWD/sublane
  install_port=8080
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version|--dir|--port)
        [[ $# -ge 2 && -n $2 ]] || fail "Missing value for $1."
        case "$1" in
          --version) install_version=${2#v}; validate_version ;;
          --dir) install_dir=$2 ;;
          --port) install_port=$2 ;;
        esac
        shift 2
        ;;
      --help|-h) usage; return ;;
      *) fail "Unknown argument: $1. Use --help." ;;
    esac
  done

  [[ $install_port =~ ^[1-9][0-9]{0,4}$ ]] || fail 'Port must be an integer from 1 to 65535.'
  [[ $install_port -le 65535 ]] || fail 'Port must be an integer from 1 to 65535.'
  [[ $install_dir != *$'\n'* && $install_dir != *$'\r'* ]] || fail 'Directory cannot contain line breaks.'
  [[ $install_dir == /* ]] || install_dir=$PWD/$install_dir
  [[ ! -e $install_dir && ! -L $install_dir ]] || fail "Directory already exists: $install_dir. Configuration and data were left unchanged."

  local dependency
  for dependency in curl docker awk mktemp; do
    command -v "$dependency" >/dev/null || fail "Install $dependency first."
  done
  if [[ -z $install_version ]]; then
    command -v jq >/dev/null || fail 'Install jq for automatic version selection, or pass --version explicitly.'
  fi
  if command -v sha256sum >/dev/null; then
    install_hash=(sha256sum)
  elif command -v shasum >/dev/null; then
    install_hash=(shasum -a 256)
  else
    fail 'Install sha256sum or shasum first.'
  fi
  docker compose version >/dev/null || fail 'Docker Compose is required.'
  docker info >/dev/null 2>&1 || fail 'Docker is not running or the current user cannot access it.'

  umask 077
  install_stage=$(mktemp -d "${TMPDIR:-/tmp}/sublane-install.XXXXXXXX")
  trap cleanup EXIT
  trap 'exit 130' INT TERM
  if [[ -z $install_version ]]; then resolve_version; fi
  validate_version
  install_release="https://github.com/murongg/SubLane/releases/download/v$install_version"
  install_image="ghcr.io/murongg/sublane:$install_version"
  # Persist a distinct project name so a same-named directory cannot reuse another instance's volume.
  install_project="sublane-$RANDOM-$RANDOM-$$"

  printf 'Downloading deployment files for v%s...\n' "$install_version"
  download docker.compose.yaml
  download SHA256SUMS
  local expected actual
  expected=$(awk '$2 == "docker.compose.yaml" { print $1 }' "$install_stage/SHA256SUMS")
  [[ $expected =~ ^[0-9a-f]{64}$ ]] || fail 'Release checksums must contain exactly one docker.compose.yaml entry.'
  actual=$("${install_hash[@]}" "$install_stage/docker.compose.yaml")
  [[ ${actual%% *} == "$expected" ]] || fail 'Compose checksum mismatch; installation stopped.'

  cat > "$install_stage/.env" <<ENV
COMPOSE_PROJECT_NAME=$install_project
SUBLANE_IMAGE=$install_image
SUBLANE_BIND_ADDRESS=127.0.0.1
SUBLANE_PORT=$install_port
SUBLANE_LOG_LEVEL=info
# Set this when configuring an HTTPS reverse proxy:
# SUBLANE_PUBLIC_URL=https://sublane.example.com
ENV
  compose "$install_stage" config --quiet
  printf 'Pulling %s...\n' "$install_image"
  compose "$install_stage" pull || fail 'Image pull failed; no installation was created.'

  mkdir -p -- "$(dirname "$install_dir")"
  # Claim the directory atomically only after verification; simultaneous installers must not overwrite it.
  mkdir -- "$install_dir" || fail "Could not create $install_dir; existing files were left unchanged."
  cp "$install_stage/docker.compose.yaml" "$install_stage/.env" "$install_dir/"
  if ! compose "$install_dir" up --detach --wait --wait-timeout 90 --no-build; then
    printf 'Configuration and any container data are retained in the deployment at %s.\n' "$install_dir" >&2
    printf 'Inspect it with: cd %q && docker compose -f docker.compose.yaml logs --tail=100 sublane\n' "$install_dir" >&2
    fail 'SubLane did not become healthy.'
  fi
  printf '\nInstallation complete: %s\nOpen http://127.0.0.1:%s to create the administrator.\n' "$install_dir" "$install_port"
  printf 'Manage this deployment: cd %q && docker compose -f docker.compose.yaml ps\n' "$install_dir"
}

# Invoke only after the complete function definition has arrived when used through curl | bash.
main "$@"
